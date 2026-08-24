package driver

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCreateVolumeIsIdempotent(t *testing.T) {
	d := testDriver()
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
	d := testDriver()
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
	d := testDriver()
	for range 2 {
		if _, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: "missing"}); err != nil {
			t.Fatal(err)
		}
	}
}

func testDriver() *Driver {
	return New(Config{NodeID: "node-a", Version: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func testCapabilities() []*csi.VolumeCapability {
	return []*csi.VolumeCapability{{
		AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
	}}
}
