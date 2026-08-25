package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (d *Driver) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: []*csi.ControllerServiceCapability{
		{Type: &csi.ControllerServiceCapability_Rpc{Rpc: &csi.ControllerServiceCapability_RPC{Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME}}},
		{Type: &csi.ControllerServiceCapability_Rpc{Rpc: &csi.ControllerServiceCapability_RPC{Type: csi.ControllerServiceCapability_RPC_GET_CAPACITY}}},
	}}, nil
}

func (d *Driver) CreateVolume(_ context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}
	if req.GetVolumeContentSource() != nil {
		return nil, status.Error(codes.InvalidArgument, "volume content sources are not supported")
	}
	if err := validateCapabilities(req.GetVolumeCapabilities()); err != nil {
		return nil, err
	}
	capacity, err := requestedCapacity(req.GetCapacityRange())
	if err != nil {
		return nil, err
	}
	if req.GetParameters()["quotas"] == "true" {
		return nil, status.Error(codes.InvalidArgument, "quotas are not supported yet")
	}
	if !d.topologyAllowed(req.GetAccessibilityRequirements()) {
		return nil, status.Error(codes.ResourceExhausted, "volume is not accessible from the requested topology")
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	id := volumeID(req.GetName())
	if existing, err := d.store.load(id); err == nil {
		if existing.Name != req.GetName() || !capacityCompatible(existing.CapacityBytes, req.GetCapacityRange()) || !equalParameters(existing.Parameters, req.GetParameters()) {
			return nil, status.Error(codes.AlreadyExists, "volume name already exists with different parameters")
		}
		if err := d.store.ensureDirectory(id); err != nil {
			return nil, status.Errorf(codes.Internal, "recreate volume directory: %v", err)
		}
		return d.createVolumeResponse(existing), nil
	} else if !isNotExist(err) {
		return nil, status.Errorf(codes.Internal, "read volume metadata: %v", err)
	}

	v := volumeMetadata{ID: id, Name: req.GetName(), NodeID: d.config.NodeID, CapacityBytes: capacity, Parameters: cloneMap(req.GetParameters())}
	if err := d.store.create(v); err != nil {
		return nil, status.Errorf(codes.Internal, "create volume: %v", err)
	}
	d.log.Info("created volume", "volume_id", id, "capacity_bytes", capacity)
	return d.createVolumeResponse(v), nil
}

func (d *Driver) createVolumeResponse(v volumeMetadata) *csi.CreateVolumeResponse {
	return &csi.CreateVolumeResponse{Volume: &csi.Volume{VolumeId: v.ID, CapacityBytes: v.CapacityBytes, AccessibleTopology: []*csi.Topology{d.topology()}}}
}

func (d *Driver) DeleteVolume(_ context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	if !validVolumeID(req.GetVolumeId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid volume id")
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.store.delete(req.GetVolumeId()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete volume: %v", err)
	}
	d.log.Info("deleted volume", "volume_id", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ValidateVolumeCapabilities(_ context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}
	if !validVolumeID(req.GetVolumeId()) {
		return nil, status.Error(codes.InvalidArgument, "invalid volume id")
	}
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if _, err := d.store.load(req.GetVolumeId()); err != nil {
		if isNotExist(err) {
			return nil, status.Error(codes.NotFound, "volume not found")
		}
		return nil, status.Errorf(codes.Internal, "read volume metadata: %v", err)
	}
	if err := validateCapabilities(req.GetVolumeCapabilities()); err != nil {
		return &csi.ValidateVolumeCapabilitiesResponse{Message: err.Error()}, nil
	}
	return &csi.ValidateVolumeCapabilitiesResponse{Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: req.GetVolumeCapabilities(), Parameters: cloneMap(req.GetParameters())}}, nil
}

func (d *Driver) GetCapacity(_ context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	if err := d.checkFence(); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	if req.GetAccessibleTopology() != nil && req.GetAccessibleTopology().GetSegments()[topologyKey] != d.config.NodeID {
		return &csi.GetCapacityResponse{}, nil
	}
	available, err := availableBytes(d.config.BasePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get filesystem capacity: %v", err)
	}
	return &csi.GetCapacityResponse{AvailableCapacity: available}, nil
}

func (d *Driver) topologyAllowed(requirement *csi.TopologyRequirement) bool {
	if requirement == nil || len(requirement.GetRequisite()) == 0 {
		return true
	}
	for _, topology := range requirement.GetRequisite() {
		if topology.GetSegments()[topologyKey] == d.config.NodeID {
			return true
		}
	}
	return false
}

func volumeID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "vol-" + hex.EncodeToString(sum[:16])
}
