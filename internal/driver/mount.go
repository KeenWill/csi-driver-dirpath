package driver

import (
	"bufio"
	"errors"
	"fmt"
	"os"
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
		if err := unix.Mount("", target, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
			_ = unix.Unmount(target, unix.MNT_DETACH)
			return err
		}
	}
	return nil
}

func (linuxMounter) Unmount(target string) error {
	err := unix.Unmount(target, 0)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOENT) {
		return nil
	}
	return err
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
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 4 && unescapeMountInfo(fields[4]) == target {
			return true, nil
		}
	}
	return false, scanner.Err()
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

func availableBytes(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if available > uint64(^uint64(0)>>1) {
		return 0, fmt.Errorf("available capacity overflows int64")
	}
	return int64(available), nil
}
