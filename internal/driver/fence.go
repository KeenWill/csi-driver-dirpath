package driver

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type fence struct {
	basePath string
	token    string
	mode     string
	id       string
}

func newFence(basePath, token, mode, id string) *fence {
	return &fence{basePath: basePath, token: token, mode: mode, id: id}
}

func (f *fence) validateConfig() error {
	switch f.mode {
	case "marker":
		return nil
	case "fsid", "device":
		if f.id == "" {
			return fmt.Errorf("fence-id is required in %s mode", f.mode)
		}
		return nil
	default:
		return fmt.Errorf("fence-mode must be marker, fsid, or device")
	}
}

func (f *fence) validate() error {
	marker, err := os.ReadFile(filepath.Join(f.basePath, ".dirpath-fence"))
	if err != nil {
		return fmt.Errorf("read marker: %w", err)
	}
	if string(marker) != f.token {
		return fmt.Errorf("marker token does not match")
	}

	switch f.mode {
	case "marker":
		return nil
	case "fsid":
		actual, err := filesystemID(f.basePath)
		if err != nil {
			return err
		}
		if actual != f.id {
			return fmt.Errorf("filesystem id %q does not match %q", actual, f.id)
		}
	case "device":
		actual, err := deviceID(f.basePath)
		if err != nil {
			return err
		}
		if actual != f.id {
			return fmt.Errorf("device %q does not match %q", actual, f.id)
		}
	}
	return nil
}

func filesystemID(path string) (string, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return "", fmt.Errorf("statfs: %w", err)
	}
	return fmt.Sprintf("%08x:%08x", uint32(stat.Fsid.Val[0]), uint32(stat.Fsid.Val[1])), nil
}

func deviceID(path string) (string, error) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return "", fmt.Errorf("stat: %w", err)
	}
	device := uint64(stat.Dev)
	return fmt.Sprintf("%d:%d", unix.Major(device), unix.Minor(device)), nil
}
