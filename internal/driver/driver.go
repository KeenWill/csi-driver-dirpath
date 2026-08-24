package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	driverName  = "csi.dirpath.dev"
	topologyKey = driverName + "/node"
)

type Config struct {
	Endpoint string
	NodeID   string
	Version  string
}

type volume struct {
	id         string
	name       string
	capacity   int64
	parameters map[string]string
}

type Driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer

	config Config
	log    *slog.Logger

	mu     sync.Mutex
	byID   map[string]volume
	byName map[string]string
}

func New(config Config, logger *slog.Logger) *Driver {
	return &Driver{
		config: config,
		log:    logger,
		byID:   make(map[string]volume),
		byName: make(map[string]string),
	}
}

func (d *Driver) Run() error {
	network, address, err := parseEndpoint(d.config.Endpoint)
	if err != nil {
		return err
	}
	if network == "unix" {
		if err := os.MkdirAll(filepath.Dir(address), 0o750); err != nil {
			return fmt.Errorf("create socket directory: %w", err)
		}
		if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale socket: %w", err)
		}
	}

	listener, err := net.Listen(network, address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.config.Endpoint, err)
	}
	defer listener.Close()

	server := grpc.NewServer()
	csi.RegisterIdentityServer(server, d)
	csi.RegisterControllerServer(server, d)
	csi.RegisterNodeServer(server, d)
	d.log.Info("serving CSI", "endpoint", d.config.Endpoint, "node", d.config.NodeID)
	return server.Serve(listener)
}

func parseEndpoint(endpoint string) (string, string, error) {
	switch {
	case strings.HasPrefix(endpoint, "unix://"):
		return "unix", strings.TrimPrefix(endpoint, "unix://"), nil
	case strings.HasPrefix(endpoint, "unix:"):
		return "unix", strings.TrimPrefix(endpoint, "unix:"), nil
	default:
		return "", "", fmt.Errorf("unsupported endpoint %q", endpoint)
	}
}

func (d *Driver) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{Name: driverName, VendorVersion: d.config.Version}, nil
}

func (d *Driver) GetPluginCapabilities(context.Context, *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{Capabilities: []*csi.PluginCapability{
		{Type: &csi.PluginCapability_Service_{Service: &csi.PluginCapability_Service{Type: csi.PluginCapability_Service_CONTROLLER_SERVICE}}},
		{Type: &csi.PluginCapability_Service_{Service: &csi.PluginCapability_Service{Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS}}},
	}}, nil
}

func (d *Driver) Probe(context.Context, *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}

func (d *Driver) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: []*csi.ControllerServiceCapability{
		{Type: &csi.ControllerServiceCapability_Rpc{Rpc: &csi.ControllerServiceCapability_RPC{Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME}}},
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

	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.byName[req.GetName()]; ok {
		v := d.byID[id]
		if v.capacity != capacity || !equalParameters(v.parameters, req.GetParameters()) {
			return nil, status.Error(codes.AlreadyExists, "volume name already exists with different parameters")
		}
		return d.createVolumeResponse(v), nil
	}

	sum := sha256.Sum256([]byte(req.GetName()))
	id := "vol-" + hex.EncodeToString(sum[:16])
	v := volume{id: id, name: req.GetName(), capacity: capacity, parameters: cloneMap(req.GetParameters())}
	d.byID[id] = v
	d.byName[v.name] = id
	return d.createVolumeResponse(v), nil
}

func (d *Driver) createVolumeResponse(v volume) *csi.CreateVolumeResponse {
	return &csi.CreateVolumeResponse{Volume: &csi.Volume{
		VolumeId:      v.id,
		CapacityBytes: v.capacity,
		AccessibleTopology: []*csi.Topology{{Segments: map[string]string{
			topologyKey: d.config.NodeID,
		}}},
	}}
}

func (d *Driver) DeleteVolume(_ context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, ok := d.byID[req.GetVolumeId()]; ok {
		delete(d.byName, v.name)
		delete(d.byID, v.id)
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ValidateVolumeCapabilities(_ context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}
	d.mu.Lock()
	_, ok := d.byID[req.GetVolumeId()]
	d.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "volume not found")
	}
	if err := validateCapabilities(req.GetVolumeCapabilities()); err != nil {
		return &csi.ValidateVolumeCapabilitiesResponse{Message: err.Error()}, nil
	}
	return &csi.ValidateVolumeCapabilitiesResponse{Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
		VolumeCapabilities: req.GetVolumeCapabilities(),
		Parameters:         cloneMap(req.GetParameters()),
	}}, nil
}

func (d *Driver) NodeGetCapabilities(context.Context, *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{}, nil
}

func (d *Driver) NodeGetInfo(context.Context, *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.config.NodeID,
		AccessibleTopology: &csi.Topology{Segments: map[string]string{
			topologyKey: d.config.NodeID,
		}},
	}, nil
}

func (d *Driver) NodePublishVolume(_ context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
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
	if err := os.MkdirAll(req.GetTargetPath(), 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "create target path: %v", err)
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) NodeUnpublishVolume(_ context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is required")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	if err := os.Remove(req.GetTargetPath()); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "remove target path: %v", err)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

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
