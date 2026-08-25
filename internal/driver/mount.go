package driver

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type mounter interface {
	Mount(source, target string, readOnly bool) error
	Unmount(target string) error
	Mounted(target string) (bool, bool, error)
}

type linuxMounter struct{}

func (linuxMounter) Mount(source, target string, readOnly bool) error {
	if err := unix.Mount(source, target, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return err
	}
	if readOnly {
		if err := makeMountReadOnly(target); err != nil {
			_ = unix.Unmount(target, unix.MNT_DETACH)
			return err
		}
	}
	return nil
}

func (linuxMounter) Unmount(target string) error {
	return unmountTree(target, unmountOps{
		mountPoints: mountedPathsAtOrBelow,
		unmount:     func(path string) error { return unix.Unmount(path, 0) },
	})
}

func (linuxMounter) Mounted(target string) (bool, bool, error) {
	mounted, err := mountpointInProc(target)
	if err != nil || !mounted {
		return mounted, false, err
	}
	return true, false, nil
}

type noopMounter struct{}

func (noopMounter) Mount(string, string, bool) error { return nil }
func (noopMounter) Unmount(string) error             { return nil }
func (noopMounter) Mounted(string) (bool, bool, error) {
	return false, false, nil
}

func mountpointInProc(target string) (bool, error) {
	resolved, err := filepath.EvalSymlinks(target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	mounts, err := mountedPathsAtOrBelow(resolved)
	if err != nil {
		return false, err
	}
	for _, mount := range mounts {
		if mount == filepath.Clean(resolved) {
			return true, nil
		}
	}
	return false, nil
}

type readOnlyMountOps struct {
	setRecursive func(string) error
	mountPoints  func(string) ([]string, error)
	remount      func(string) error
}

type unmountOps struct {
	mountPoints func(string) ([]string, error)
	unmount     func(string) error
}

func unmountTree(target string, ops unmountOps) error {
	mounts, err := ops.mountPoints(target)
	if err != nil {
		return fmt.Errorf("list recursive bind mounts: %w", err)
	}
	sortMountsDeepestFirst(mounts)
	for _, mount := range mounts {
		if err := ops.unmount(mount); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("unmount %s: %w", mount, err)
		}
	}
	return nil
}

func makeMountReadOnly(target string) error {
	ops := readOnlyMountOps{
		setRecursive: func(path string) error {
			return unix.MountSetattr(unix.AT_FDCWD, path, unix.AT_RECURSIVE, &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY})
		},
		mountPoints: mountedPathsAtOrBelow,
		remount: func(path string) error {
			return unix.Mount("", path, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, "")
		},
	}
	return applyRecursiveReadOnly(target, ops)
}

func applyRecursiveReadOnly(target string, ops readOnlyMountOps) error {
	if err := ops.setRecursive(target); err == nil {
		return nil
	} else if !errors.Is(err, unix.ENOSYS) && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.EOPNOTSUPP) {
		return fmt.Errorf("set recursive read-only mount attribute: %w", err)
	}
	mounts, err := ops.mountPoints(target)
	if err != nil {
		return fmt.Errorf("list recursive bind mounts: %w", err)
	}
	if len(mounts) == 0 {
		return fmt.Errorf("recursive bind mount is absent from mountinfo")
	}
	for _, mount := range mounts {
		if err := ops.remount(mount); err != nil {
			return fmt.Errorf("remount %s read-only: %w", mount, err)
		}
	}
	return nil
}

func mountedPathsAtOrBelow(root string) ([]string, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return mountPointsAtOrBelow(file, resolved)
}

func mountPointsAtOrBelow(reader io.Reader, root string) ([]string, error) {
	root = filepath.Clean(root)
	var mounts []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) <= 4 {
			continue
		}
		mount := filepath.Clean(unescapeMountInfo(fields[4]))
		relative, err := filepath.Rel(root, mount)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		mounts = append(mounts, mount)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sortMountsDeepestFirst(mounts)
	return mounts, nil
}

func sortMountsDeepestFirst(mounts []string) {
	sort.SliceStable(mounts, func(i, j int) bool { return len(mounts[i]) > len(mounts[j]) })
}

func sameFile(source, target string) (bool, error) {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return false, err
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		return false, err
	}
	return os.SameFile(sourceInfo, targetInfo), nil
}

func unescapeMountInfo(path string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(path)
}

func availableBytes(path BasePath) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(string(path), &stat); err != nil {
		return 0, err
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if available > uint64(^uint64(0)>>1) {
		return 0, fmt.Errorf("available capacity overflows int64")
	}
	return int64(available), nil
}
