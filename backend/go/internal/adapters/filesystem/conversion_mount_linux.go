//go:build linux

package filesystem

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func conversionMount(path string) (string, error) {
	var stat unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &stat)
	return conversionMountFromStat(
		path,
		stat,
		err,
		func() ([]byte, error) { return os.ReadFile("/proc/self/mountinfo") },
	)
}

func conversionMountFromStat(
	path string,
	stat unix.Statx_t,
	statErr error,
	readMountInfo func() ([]byte, error),
) (string, error) {
	if statErr == nil && stat.Mask&unix.STATX_MNT_ID != 0 {
		return strconv.FormatUint(stat.Mnt_id, 10), nil
	}
	if statErr != nil && !errors.Is(statErr, unix.ENOSYS) && !errors.Is(statErr, unix.EINVAL) &&
		!errors.Is(statErr, unix.EOPNOTSUPP) {
		return "", statErr
	}
	data, err := readMountInfo()
	if err != nil {
		return "", fmt.Errorf("read mount identity: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return mountFromInfo(string(data), resolved)
}

// mountFromInfo chooses the deepest mount point with a path-component boundary.
// The last equally deep entry is the visible mount when mounts are stacked.
func mountFromInfo(data, path string) (string, error) {
	best, id := -1, ""
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			return "", fmt.Errorf("invalid mountinfo")
		}
		if _, err := strconv.ParseUint(fields[0], 10, 64); err != nil {
			return "", fmt.Errorf("invalid mount ID: %w", err)
		}
		mount, err := unescapeMount(fields[4])
		if err != nil {
			return "", err
		}
		mount = filepath.Clean(mount)
		if path != mount && !strings.HasPrefix(path, strings.TrimSuffix(mount, "/")+"/") {
			continue
		}
		if len(mount) >= best {
			best, id = len(mount), fields[0]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("mount identity unavailable: %s", path)
	}
	return id, nil
}

func unescapeMount(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		if i+3 >= len(value) {
			return "", fmt.Errorf("invalid mount escape")
		}
		n, err := strconv.ParseUint(value[i+1:i+4], 8, 8)
		if err != nil {
			return "", err
		}
		out.WriteByte(byte(n))
		i += 3
	}
	return out.String(), nil
}
