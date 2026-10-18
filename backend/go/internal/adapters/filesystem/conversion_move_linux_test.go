//go:build linux

package filesystem_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
)

func TestConversionLinkEEXISTDistinguishesSuccessConflictAndUnknown(t *testing.T) {
	for _, scenario := range []string{"same", "different", "unknown", "unlink-fails"} {
		t.Run(scenario, func(t *testing.T) {
			testLinkOutcome(t, scenario)
		})
	}
}

func TestMountInfoBoundariesEscapesAndBindMounts(t *testing.T) {
	info := "1 0 8:1 / / rw - ext4 /dev/a rw\n2 1 8:1 / /library rw - ext4 /dev/a rw\n3 1 8:1 / /library/bind rw - ext4 /dev/a rw\n4 1 8:1 / /space\\040and\\134slash rw - ext4 /dev/a rw\n"
	for path, want := range map[string]string{"/library/a": "2", "/library2/a": "1", "/library/bind/a": "3", "/space and\\slash/a": "4", "/": "1"} {
		got, err := filesystem.MountFromInfoForTest(info, path)
		if err != nil || got != want {
			t.Fatalf("mount %s = %s, %v; want %s", path, got, err, want)
		}
	}
	if _, err := filesystem.MountFromInfoForTest("broken", "/a"); err == nil {
		t.Fatal("invalid mountinfo accepted")
	}
	_, err := filesystem.ConversionMoveOnMountsForTest(
		"/library/file",
		"/library/bind/file",
		func(path string) (string, error) { return filesystem.MountFromInfoForTest(info, path) },
	)
	if !errors.Is(err, execute.ErrMoveUnsupported) {
		t.Fatalf("same-device bind mount move = %v", err)
	}
	if mount, err := filesystem.ConversionMountForTest(t.TempDir()); err != nil || mount == "" {
		t.Fatalf("real mount ID = %s, %v", mount, err)
	}
}

func testLinkOutcome(t *testing.T, scenario string) {
	t.Helper()
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := func(src, dst string) error {
		if scenario == "different" {
			if err := os.WriteFile(dst, []byte("other"), 0o600); err != nil {
				return err
			}
		} else if err := os.Link(src, dst); err != nil {
			return err
		}
		return os.ErrExist // Simulate NFS retransmission after the link landed.
	}
	inspect := func(path string) (os.FileInfo, error) {
		if scenario == "unknown" && path == target {
			return nil, os.ErrPermission
		}
		return os.Lstat(path)
	}
	unlink := os.Remove
	if scenario == "unlink-fails" {
		unlink = func(string) error { return os.ErrPermission }
	}
	result, err := filesystem.ConversionLinkMoveForTest(source, target, link, unlink, inspect)
	switch scenario {
	case "same":
		if err != nil || !result.TargetCreated || !result.SourceRemoved || result.Unverified {
			t.Fatalf("result = %+v, %v", result, err)
		}
	case "different":
		if !errors.Is(err, os.ErrExist) || result.TargetCreated || result.Unverified {
			t.Fatalf("result = %+v, %v", result, err)
		}
	case "unknown":
		if err == nil || !result.Unverified {
			t.Fatalf("result = %+v, %v", result, err)
		}
	case "unlink-fails":
		if err == nil || !result.TargetCreated || result.SourceRemoved {
			t.Fatalf("result = %+v, %v", result, err)
		}
	}
	if scenario != "same" {
		if _, err := os.Stat(source); err != nil {
			t.Fatal("source copy lost", err)
		}
	}
}

func TestConversionMountStatxFallbackAndFailure(t *testing.T) {
	path := t.TempDir()
	read := func() ([]byte, error) { return []byte("7 0 8:1 / / rw - ext4 /dev/a rw\n"), nil }
	for _, statErr := range []error{nil, unix.ENOSYS, unix.EINVAL, unix.EOPNOTSUPP} {
		id, err := filesystem.ConversionMountFromStatForTest(path, unix.Statx_t{}, statErr, read)
		if err != nil || id != "7" {
			t.Fatalf("fallback %v = %s, %v", statErr, id, err)
		}
	}
	id, err := filesystem.ConversionMountFromStatForTest(
		path,
		unix.Statx_t{Mask: unix.STATX_MNT_ID, Mnt_id: 42},
		nil,
		func() ([]byte, error) { t.Fatal("fallback used despite statx identity"); return nil, nil },
	)
	if err != nil || id != "42" {
		t.Fatalf("statx = %s, %v", id, err)
	}
	if _, err := filesystem.ConversionMountFromStatForTest(
		path,
		unix.Statx_t{},
		unix.EACCES,
		read,
	); !errors.Is(
		err,
		unix.EACCES,
	) {
		t.Fatal(err)
	}
	if _, err := filesystem.ConversionMountFromStatForTest(
		path,
		unix.Statx_t{},
		nil,
		func() ([]byte, error) { return nil, os.ErrPermission },
	); err == nil {
		t.Fatal("unreliable mount identity accepted")
	}
}
