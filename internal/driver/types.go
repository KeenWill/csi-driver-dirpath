package driver

import (
	"fmt"
	"path/filepath"
	"regexp"
)

var volumeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type VolumeID string

type ProjectID uint32

type NodeID string

type BasePath string

type FenceToken string

type FenceMode string

const (
	MarkerFence     FenceMode = "marker"
	FilesystemFence FenceMode = "fsid"
	DeviceFence     FenceMode = "device"
)

type MountMode string

const (
	RealMounts MountMode = "real"
	NoopMounts MountMode = "noop"
)

func ParseNodeID(value string) (NodeID, error) {
	if value == "" {
		return "", fmt.Errorf("must not be empty")
	}
	return NodeID(value), nil
}

func ParseBasePath(value string) (BasePath, error) {
	if value == "" {
		return "", fmt.Errorf("must not be empty")
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("must be absolute")
	}
	return BasePath(filepath.Clean(value)), nil
}

func ParseFenceToken(value string) (FenceToken, error) {
	if value == "" {
		return "", fmt.Errorf("must not be empty")
	}
	return FenceToken(value), nil
}

func ParseFenceMode(value string) (FenceMode, error) {
	mode := FenceMode(value)
	switch mode {
	case MarkerFence, FilesystemFence, DeviceFence:
		return mode, nil
	default:
		return "", fmt.Errorf("must be marker, fsid, or device")
	}
}

func ParseMountMode(value string) (MountMode, error) {
	mode := MountMode(value)
	switch mode {
	case RealMounts, NoopMounts:
		return mode, nil
	default:
		return "", fmt.Errorf("must be real or noop")
	}
}

func parseVolumeID(value string) (VolumeID, error) {
	if !volumeIDPattern.MatchString(value) {
		return "", fmt.Errorf("invalid volume id %q", value)
	}
	return VolumeID(value), nil
}
