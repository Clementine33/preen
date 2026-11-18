package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderToOpenAdmitsOnlyExistingAbsoluteDirectories(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		path string
		ok   bool
	}{
		"directory": {dir, true},
		"relative":  {"relative/dir", false},
		"file":      {file, false},
		"missing":   {filepath.Join(dir, "missing"), false},
	} {
		if err := folderToOpen(tc.path); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok = %v", name, err, tc.ok)
		}
	}
}
