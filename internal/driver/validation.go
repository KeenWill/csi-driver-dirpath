package driver

import (
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func validateCapabilities(capabilities []*csi.VolumeCapability) error {
	for _, capability := range capabilities {
		if capability == nil || capability.GetAccessMode() == nil {
			return status.Error(codes.InvalidArgument, "access mode is required")
		}
		if capability.GetMount() == nil {
			return status.Error(codes.InvalidArgument, "only mount volumes are supported")
		}
		if capability.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return status.Error(codes.InvalidArgument, "only SINGLE_NODE_WRITER is supported")
		}
	}
	return nil
}

func requestedCapacity(capacity *csi.CapacityRange) (int64, error) {
	if capacity == nil {
		return 0, nil
	}
	if capacity.GetRequiredBytes() < 0 || capacity.GetLimitBytes() < 0 {
		return 0, status.Error(codes.InvalidArgument, "capacity must not be negative")
	}
	if capacity.GetLimitBytes() > 0 && capacity.GetRequiredBytes() > capacity.GetLimitBytes() {
		return 0, status.Error(codes.InvalidArgument, "required capacity exceeds limit")
	}
	return capacity.GetRequiredBytes(), nil
}

func capacityCompatible(existing int64, requested *csi.CapacityRange) bool {
	if requested == nil {
		return true
	}
	return existing >= requested.GetRequiredBytes() && (requested.GetLimitBytes() == 0 || existing <= requested.GetLimitBytes())
}

func cloneMap(source map[string]string) map[string]string {
	destination := make(map[string]string, len(source))
	for key, value := range source {
		destination[key] = value
	}
	return destination
}

func equalParameters(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func isNotExist(err error) bool {
	return os.IsNotExist(err)
}
