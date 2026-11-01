package librarypath_test

import (
	"fmt"
	"strings"
	"syscall"
	"testing"

	"github.com/Clementine33/preen/backend/internal/librarypath"
)

func TestNameFits(t *testing.T) {
	const posix, drive, unc = "/mnt/onsei", `D:\Music`, `\\nas\media`
	cases := []struct {
		root, name string
		want       bool
	}{
		{posix, strings.Repeat("a", 255), true},
		{posix, strings.Repeat("a", 256), false},
		{posix, strings.Repeat("あ", 85), true},
		{posix, strings.Repeat("あ", 86), false},
		{drive, strings.Repeat("あ", 255), true},
		{drive, strings.Repeat("あ", 256), false},
		{drive, strings.Repeat("😀", 127), true},
		{drive, strings.Repeat("😀", 128), false},
		{unc, strings.Repeat("あ", 85), true},
		{unc, strings.Repeat("あ", 86), false},
		{unc, strings.Repeat("a", 256), false},
	}
	for _, c := range cases {
		if got := librarypath.NameFits(c.root, c.name); got != c.want {
			t.Errorf("NameFits(%q, %d runes of %q) = %v, want %v",
				c.root, len([]rune(c.name)), []rune(c.name)[0:1], got, c.want)
		}
	}
}

func TestIsNameTooLong(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("rename: %w", syscall.ENAMETOOLONG),
		fmt.Errorf("rename: %w", syscall.Errno(206)),
		fmt.Errorf("recover: %w", librarypath.ErrNameTooLong),
	} {
		if !librarypath.IsNameTooLong(err) {
			t.Errorf("IsNameTooLong(%v) = false", err)
		}
	}
	if librarypath.IsNameTooLong(syscall.ENOENT) {
		t.Error("ENOENT is not a name refusal")
	}
}
