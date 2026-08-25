package driver

import (
	"context"
	"errors"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (d *Driver) NodeGetCapabilities(context.Context, *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: []*csi.NodeServiceCapability{
		{Type: &csi.NodeServiceCapability_Rpc{Rpc: &csi.NodeServiceCapability_RPC{Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS}}},
	}}, nil
}

func (d *Driver) NodeGetInfo(context.Context, *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return &csi.NodeGetInfoResponse{NodeId: d.config.NodeID, AccessibleTopology: d.topology()}, nil
}

func (d *Driver) NodePublishVolume(_ context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	if !validVolumeID(req.GetVolumeId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid volume id")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability is required")
	}
	if err := validateCapabilities([]*csi.VolumeCapability{req.GetVolumeCapability()}); err != nil {
		return nil, err
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	unlock := d.volumeLocks.lock(req.GetVolumeId())
	defer unlock()
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if _, err := d.store.load(req.GetVolumeId()); err != nil {
		if isNotExist(err) {
			d.log.Error("volume metadata is missing; recreating empty scratch volume", "volume_id", req.GetVolumeId())
		} else {
			return nil, status.Errorf(codes.Internal, "read volume metadata: %v", err)
		}
	}
	source := d.store.volumePath(req.GetVolumeId())
	if _, err := os.Stat(source); os.IsNotExist(err) {
		d.log.Error("volume directory is missing; recreating empty scratch volume", "volume_id", req.GetVolumeId())
		if err := d.checkFence(); err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if err := d.store.ensureDirectory(req.GetVolumeId()); err != nil {
			return nil, status.Errorf(codes.Internal, "recreate volume directory: %v", err)
		}
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "inspect volume directory: %v", err)
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if err := os.MkdirAll(req.GetTargetPath(), 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "create target path: %v", err)
	}
	mounted, _, err := d.mounter.Mounted(req.GetTargetPath())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "inspect target mount: %v", err)
	}
	if mounted {
		same, err := sameFile(source, req.GetTargetPath())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "verify target mount: %v", err)
		}
		if !same {
			return nil, status.Error(codes.FailedPrecondition, "target path is already mounted from another source")
		}
		return &csi.NodePublishVolumeResponse{}, nil
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if err := d.mounter.Mount(source, req.GetTargetPath(), req.GetReadonly()); err != nil {
		return nil, status.Errorf(codes.Internal, "bind mount volume: %v", err)
	}
	d.log.Info("published volume", "volume_id", req.GetVolumeId(), "target_path", req.GetTargetPath())
	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) NodeUnpublishVolume(_ context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	if !validVolumeID(req.GetVolumeId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid volume id")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	unlock := d.volumeLocks.lock(req.GetVolumeId())
	defer unlock()
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if err := d.mounter.Unmount(req.GetTargetPath()); err != nil {
		return nil, status.Errorf(codes.Internal, "unmount target: %v", err)
	}
	if err := os.Remove(req.GetTargetPath()); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "remove target path: %v", err)
	}
	d.log.Info("unpublished volume", "volume_id", req.GetVolumeId(), "target_path", req.GetTargetPath())
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *Driver) NodeGetVolumeStats(_ context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	if !validVolumeID(req.GetVolumeId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid volume id")
	}
	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path is required")
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	unlock := d.volumeLocks.lock(req.GetVolumeId())
	defer unlock()
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(req.GetVolumePath(), &stat); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, status.Error(codes.NotFound, "volume path not found")
		}
		return nil, status.Errorf(codes.Internal, "stat volume: %v", err)
	}
	totalBytes := int64(stat.Blocks) * stat.Bsize
	available := int64(stat.Bavail) * stat.Bsize
	usedBytes := totalBytes - int64(stat.Bfree)*stat.Bsize
	return &csi.NodeGetVolumeStatsResponse{Usage: []*csi.VolumeUsage{
		{Unit: csi.VolumeUsage_BYTES, Total: totalBytes, Available: available, Used: usedBytes},
		{Unit: csi.VolumeUsage_INODES, Total: int64(stat.Files), Available: int64(stat.Ffree), Used: int64(stat.Files - stat.Ffree)},
	}}, nil
}
