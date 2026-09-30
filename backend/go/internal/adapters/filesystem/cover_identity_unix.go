//go:build !windows

package filesystem

import (
	"fmt"
	"os"
	"syscall"
)

func coverIdentity(info os.FileInfo, _ string) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("directory identity unavailable")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
