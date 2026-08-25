package driver

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestApplyRecursiveReadOnlyFallsBackAcrossNestedMounts(t *testing.T) {
	want := []string{"/target/nested/deep", "/target/nested", "/target"}
	var remounted []string
	err := applyRecursiveReadOnly("/target", readOnlyMountOps{
		setRecursive: func(string) error { return unix.ENOSYS },
		mountPoints:  func(string) ([]string, error) { return want, nil },
		remount: func(path string) error {
			remounted = append(remounted, path)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(remounted, want) {
		t.Fatalf("remounted paths = %v, want %v", remounted, want)
	}
}

func TestApplyRecursiveReadOnlyDoesNotMaskMountSetattrFailure(t *testing.T) {
	calledFallback := false
	err := applyRecursiveReadOnly("/target", readOnlyMountOps{
		setRecursive: func(string) error { return unix.EPERM },
		mountPoints: func(string) ([]string, error) {
			calledFallback = true
			return nil, nil
		},
	})
	if !errors.Is(err, unix.EPERM) {
		t.Fatalf("error = %v, want EPERM", err)
	}
	if calledFallback {
		t.Fatal("fallback ran for a mount_setattr permission failure")
	}
}

func TestMountPointsAtOrBelow(t *testing.T) {
	mountInfo := strings.NewReader(`
20 1 0:1 / /target rw - ext4 /dev/root rw
21 20 0:2 / /target/nested rw - tmpfs tmpfs rw
22 21 0:3 / /target/nested/deep rw - tmpfs tmpfs rw
23 1 0:4 / /target-other rw - tmpfs tmpfs rw
24 20 0:5 / /target/space\040name rw - tmpfs tmpfs rw
`)
	mounts, err := mountPointsAtOrBelow(mountInfo, "/target")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/target/nested/deep", "/target/space name", "/target/nested", "/target"}
	if !reflect.DeepEqual(mounts, want) {
		t.Fatalf("mounts = %v, want %v", mounts, want)
	}
}

func TestUnmountTreeUnmountsNestedMountsDeepestFirst(t *testing.T) {
	var unmounted []string
	err := unmountTree("/target", unmountOps{
		mountPoints: func(string) ([]string, error) {
			return []string{"/target", "/target/nested", "/target/nested/deep"}, nil
		},
		unmount: func(path string) error {
			unmounted = append(unmounted, path)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/target/nested/deep", "/target/nested", "/target"}
	if !reflect.DeepEqual(unmounted, want) {
		t.Fatalf("unmounted paths = %v, want %v", unmounted, want)
	}
}
