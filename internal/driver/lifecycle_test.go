package driver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDirectoryLifecycleAndFence(t *testing.T) {
	d := testDriver(t)
	created, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:               "pvc-lifecycle",
		CapacityRange:      &csi.CapacityRange{RequiredBytes: 4096},
		VolumeCapabilities: testCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := mustVolumeID(t, created.GetVolume().GetVolumeId())
	volumePath := d.store.volumePath(id)
	if info, err := os.Stat(volumePath); err != nil || !info.IsDir() {
		t.Fatalf("volume directory: info=%v err=%v", info, err)
	}

	for _, path := range []string{volumePath, d.store.metadataPath(id)} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(t.TempDir(), "target")
	if _, err := d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:         string(id),
		TargetPath:       target,
		VolumeCapability: testCapabilities()[0],
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(volumePath); err != nil {
		t.Fatalf("recreated volume directory: %v", err)
	}
	if _, err := d.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{VolumeId: string(id), TargetPath: target}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(string(d.config.BasePath), ".dirpath-fence"), []byte("wrong"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: string(id)}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeleteVolume with bad fence error = %v, want FailedPrecondition", err)
	}
	if _, err := os.Stat(volumePath); err != nil {
		t.Fatalf("fenced volume was touched: %v", err)
	}
}

func TestDeleteVolumeRemovesDirectoryAndMetadata(t *testing.T) {
	d := testDriver(t)
	created, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{Name: "pvc-delete", VolumeCapabilities: testCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	id := mustVolumeID(t, created.GetVolume().GetVolumeId())
	if _, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: string(id)}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{d.store.volumePath(id), d.store.metadataPath(id)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists, error=%v", path, err)
		}
	}
}

func TestReconcileDeletesOrphanAfterGracePeriod(t *testing.T) {
	d := testDriver(t)
	d.config.OrphanGracePeriod = time.Minute
	d.pvs = staticPVLister{}
	orphan := VolumeID("vol-orphan")
	if err := d.store.create(volumeMetadata{ID: orphan, Name: "orphan", NodeID: d.config.NodeID}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	d.reconcile(context.Background(), now)
	if _, err := os.Stat(d.store.volumePath(orphan)); err != nil {
		t.Fatalf("orphan removed before grace period: %v", err)
	}
	d.reconcile(context.Background(), now.Add(time.Minute+time.Second))
	if _, err := os.Stat(d.store.volumePath(orphan)); !os.IsNotExist(err) {
		t.Fatalf("orphan still exists: %v", err)
	}
}

func TestReconcileKeepsPersistentVolumeDirectory(t *testing.T) {
	d := testDriver(t)
	id := VolumeID("vol-present")
	if err := d.store.create(volumeMetadata{ID: id, Name: "present", NodeID: d.config.NodeID}); err != nil {
		t.Fatal(err)
	}
	d.pvs = staticPVLister{id: {}}
	d.reconcile(context.Background(), time.Now().Add(time.Hour))
	if _, err := os.Stat(d.store.volumePath(id)); err != nil {
		t.Fatalf("PV directory removed: %v", err)
	}
}

type staticPVLister map[VolumeID]struct{}

func (l staticPVLister) List(context.Context) (map[VolumeID]struct{}, error) {
	return l, nil
}
