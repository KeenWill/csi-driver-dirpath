package driver

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodePublishStopsAfterCanceledLockWait(t *testing.T) {
	d := testDriver(t)
	id := VolumeID("vol-canceled-publish")
	target := filepath.Join(t.TempDir(), "target")
	mounter := &countingMounter{}
	d.mounter = mounter

	releaseLock := d.volumeLocks.lock(id)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(releaseLock) }
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := d.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
			VolumeId:         string(id),
			TargetPath:       target,
			VolumeCapability: testCapabilities()[0],
		})
		result <- err
	}()

	deadline := time.Now().Add(time.Second)
	for d.volumeLocks.references(id) != 2 {
		if time.Now().After(deadline) {
			t.Fatal("NodePublishVolume did not reach the volume lock")
		}
		runtime.Gosched()
	}
	cancel()
	release()

	if err := <-result; status.Code(err) != codes.Canceled {
		t.Fatalf("NodePublishVolume error = %v, want Canceled", err)
	}
	if got := mounter.mountCalls.Load(); got != 0 {
		t.Fatalf("mount calls = %d, want 0", got)
	}
	for _, path := range []string{d.store.volumePath(id), target} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("path %s was created after cancellation: %v", path, err)
		}
	}
}

type countingMounter struct {
	mountCalls atomic.Int32
}

func (m *countingMounter) Mount(string, string, bool) error {
	m.mountCalls.Add(1)
	return nil
}

func (*countingMounter) Unmount(string) error { return nil }

func (*countingMounter) Mounted(string) (bool, bool, error) {
	return false, false, nil
}

func TestCreateVolumeRechecksFenceAfterWaitingForLock(t *testing.T) {
	d := testDriver(t)
	request := &csi.CreateVolumeRequest{Name: "pvc-fence-race", VolumeCapabilities: testCapabilities()}
	id := deriveVolumeID(request.Name)
	releaseLock := d.volumeLocks.lock(id)
	result := make(chan error, 1)
	go func() {
		_, err := d.CreateVolume(context.Background(), request)
		result <- err
	}()

	deadline := time.Now().Add(time.Second)
	for d.volumeLocks.references(id) != 2 {
		if time.Now().After(deadline) {
			releaseLock()
			t.Fatal("CreateVolume did not reach the volume lock")
		}
		runtime.Gosched()
	}
	if err := os.WriteFile(filepath.Join(string(d.config.BasePath), ".dirpath-fence"), []byte("wrong"), 0o600); err != nil {
		releaseLock()
		t.Fatal(err)
	}
	releaseLock()

	if err := <-result; status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CreateVolume error = %v, want FailedPrecondition", err)
	}
	for _, path := range []string{d.store.volumePath(id), d.store.metadataPath(id)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("path %s was written after fence failure: %v", path, err)
		}
	}
}

func TestSlowDeleteDoesNotBlockUnrelatedCreate(t *testing.T) {
	d := testDriver(t)
	slow, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{Name: "pvc-slow-delete", VolumeCapabilities: testCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	slowID := mustVolumeID(t, slow.GetVolume().GetVolumeId())
	slowPath := d.store.volumePath(slowID)
	originalRemoveAll := d.store.removeAll
	deleteStarted := make(chan struct{})
	releaseDelete := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDelete) }) }
	defer release()
	d.store.removeAll = func(path string) error {
		if path == slowPath {
			close(deleteStarted)
			<-releaseDelete
		}
		return originalRemoveAll(path)
	}

	deleteResult := make(chan error, 1)
	go func() {
		_, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: string(slowID)})
		deleteResult <- err
	}()
	<-deleteStarted

	createResult := make(chan error, 1)
	go func() {
		_, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{Name: "pvc-unrelated", VolumeCapabilities: testCapabilities()})
		createResult <- err
	}()
	select {
	case err := <-createResult:
		if err != nil {
			t.Fatalf("unrelated CreateVolume failed: %v", err)
		}
	case <-time.After(time.Second):
		release()
		t.Fatal("unrelated CreateVolume blocked behind recursive deletion")
	}

	release()
	if err := <-deleteResult; err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}
}
