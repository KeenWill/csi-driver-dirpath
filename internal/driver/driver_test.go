package driver

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCreateVolumeIsIdempotent(t *testing.T) {
	d := testDriver(t)
	req := &csi.CreateVolumeRequest{
		Name:               "pvc-1",
		CapacityRange:      &csi.CapacityRange{RequiredBytes: 1024},
		VolumeCapabilities: testCapabilities(),
	}

	first, err := d.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetVolume().GetVolumeId() != second.GetVolume().GetVolumeId() {
		t.Fatalf("volume IDs differ: %q and %q", first.GetVolume().GetVolumeId(), second.GetVolume().GetVolumeId())
	}
	if got := first.GetVolume().GetAccessibleTopology()[0].GetSegments()[topologyKey]; got != "node-a" {
		t.Fatalf("topology node = %q, want node-a", got)
	}
}

func TestCreateVolumeRejectsChangedRequest(t *testing.T) {
	d := testDriver(t)
	req := &csi.CreateVolumeRequest{Name: "pvc-1", CapacityRange: &csi.CapacityRange{RequiredBytes: 1024}, VolumeCapabilities: testCapabilities()}
	if _, err := d.CreateVolume(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	req.CapacityRange.RequiredBytes = 2048
	if _, err := d.CreateVolume(context.Background(), req); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("CreateVolume error = %v, want AlreadyExists", err)
	}
}

func TestDeleteVolumeIsIdempotent(t *testing.T) {
	d := testDriver(t)
	for range 2 {
		if _, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: "missing"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateVolumeRejectsMountFlags(t *testing.T) {
	d := testDriver(t)
	capabilities := testCapabilities()
	capabilities[0].GetMount().MountFlags = []string{"noatime"}
	request := &csi.CreateVolumeRequest{Name: "pvc-mount-flags", VolumeCapabilities: capabilities}

	if _, err := d.CreateVolume(context.Background(), request); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateVolume error = %v, want InvalidArgument", err)
	}
	if _, err := os.Stat(d.store.volumePath(deriveVolumeID(request.Name))); !os.IsNotExist(err) {
		t.Fatalf("unsupported volume was created: %v", err)
	}
}

func TestValidateVolumeCapabilitiesRejectsMountFlags(t *testing.T) {
	d := testDriver(t)
	created, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{Name: "pvc-validate-flags", VolumeCapabilities: testCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	capabilities := testCapabilities()
	capabilities[0].GetMount().MountFlags = []string{"noatime"}
	response, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:           created.GetVolume().GetVolumeId(),
		VolumeCapabilities: capabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetConfirmed() != nil || !strings.Contains(response.GetMessage(), "mount flags") {
		t.Fatalf("ValidateVolumeCapabilities response = %#v, want unconfirmed mount flags message", response)
	}
}

func TestCreateVolumeStrictlyParsesQuotas(t *testing.T) {
	for _, test := range []struct {
		name       string
		parameters map[string]string
		wantCode   codes.Code
		wantText   string
	}{
		{name: "absent", parameters: nil, wantCode: codes.OK},
		{name: "false", parameters: map[string]string{"quotas": "false"}, wantCode: codes.OK},
		{name: "true", parameters: map[string]string{"quotas": "true"}, wantCode: codes.InvalidArgument, wantText: "not supported"},
		{name: "invalid", parameters: map[string]string{"quotas": "ture"}, wantCode: codes.InvalidArgument, wantText: "must be true or false"},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := testDriver(t)
			_, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
				Name:               "pvc-quotas-" + test.name,
				Parameters:         test.parameters,
				VolumeCapabilities: testCapabilities(),
			})
			if got := status.Code(err); got != test.wantCode {
				t.Fatalf("CreateVolume code = %s, want %s (error: %v)", got, test.wantCode, err)
			}
			if test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("CreateVolume error = %v, want text %q", err, test.wantText)
			}
		})
	}
}

func TestValidateConfigRejectsInvalidReconciliationDurations(t *testing.T) {
	for _, test := range []struct {
		name     string
		interval time.Duration
		grace    time.Duration
	}{
		{name: "zero interval", interval: 0, grace: time.Minute},
		{name: "negative interval", interval: -time.Second, grace: time.Minute},
		{name: "negative grace", interval: time.Minute, grace: -time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := testDriver(t)
			d.config.ReconcileInterval = test.interval
			d.config.OrphanGracePeriod = test.grace
			if err := d.validateConfig(); err == nil {
				t.Fatal("validateConfig succeeded, want error")
			}
		})
	}
}

func testDriver(t *testing.T) *Driver {
	t.Helper()
	basePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(basePath, ".dirpath-fence"), []byte("test-fence"), 0o600); err != nil {
		t.Fatal(err)
	}
	return New(Config{
		NodeID:            "node-a",
		Version:           "test",
		BasePath:          BasePath(basePath),
		FenceToken:        "test-fence",
		MountMode:         "noop",
		OrphanGracePeriod: 10 * time.Minute,
		ReconcileInterval: time.Minute,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func mustVolumeID(t *testing.T, value string) VolumeID {
	t.Helper()
	id, err := parseVolumeID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testCapabilities() []*csi.VolumeCapability {
	return []*csi.VolumeCapability{{
		AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
	}}
}
