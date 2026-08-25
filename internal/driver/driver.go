package driver

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	driverName  = "csi.dirpath.dev"
	topologyKey = driverName + "/node"
)

type Config struct {
	Endpoint          string
	NodeID            NodeID
	Version           string
	BasePath          BasePath
	FenceToken        FenceToken
	FenceMode         FenceMode
	FenceID           string
	MountMode         MountMode
	OrphanReclaim     bool
	OrphanGracePeriod time.Duration
	ReconcileInterval time.Duration
}

type Driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer

	config  Config
	log     *slog.Logger
	fence   *fence
	store   *volumeStore
	mounter mounter
	pvs     pvLister

	volumeLocks *keyedMutex
	orphanMu    sync.Mutex
	orphanSince map[string]time.Time
}

func New(config Config, logger *slog.Logger) *Driver {
	if config.FenceMode == "" {
		config.FenceMode = MarkerFence
	}
	if config.MountMode == "" {
		config.MountMode = RealMounts
	}
	d := &Driver{
		config:      config,
		log:         logger,
		fence:       newFence(config.BasePath, config.FenceToken, config.FenceMode, config.FenceID),
		store:       newVolumeStore(config.BasePath),
		volumeLocks: newKeyedMutex(),
		orphanSince: make(map[string]time.Time),
	}
	if config.MountMode == NoopMounts {
		d.mounter = noopMounter{}
	} else {
		d.mounter = linuxMounter{}
	}
	return d
}

func (d *Driver) validateConfig() error {
	if d.config.NodeID == "" {
		return fmt.Errorf("node-id is required")
	}
	if d.config.BasePath == "" {
		return fmt.Errorf("base-path is required")
	}
	if d.config.FenceToken == "" {
		return fmt.Errorf("fence-token is required")
	}
	if d.config.MountMode != RealMounts && d.config.MountMode != NoopMounts {
		return fmt.Errorf("mount-mode must be real or noop")
	}
	if d.config.ReconcileInterval <= 0 {
		return fmt.Errorf("reconcile-interval must be positive")
	}
	if d.config.OrphanGracePeriod < 0 {
		return fmt.Errorf("orphan-grace-period must not be negative")
	}
	return d.fence.validateConfig()
}

func (d *Driver) checkFence() error {
	if err := d.fence.validate(); err != nil {
		return fmt.Errorf("mount fence: %w", err)
	}
	return nil
}

func (d *Driver) topology() *csi.Topology {
	return &csi.Topology{Segments: map[string]string{topologyKey: string(d.config.NodeID)}}
}
