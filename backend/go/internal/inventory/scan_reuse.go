package inventory

import (
	"path/filepath"
	"time"
)

type directoryReuseStore interface {
	LoadDirectoryReuse(root string, margin time.Duration) (map[string]DirectoryReuse, error)
}

func (s *Pipeline) directoryReuse(root string, full bool) (ReuseDirectory, error) {
	store, ok := s.repo.(directoryReuseStore)
	if full || !ok {
		return nil, nil
	}
	snapshot, err := store.LoadDirectoryReuse(root, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return func(path string, mtime int64) (DirectoryReuse, bool) {
		d, found := snapshot[filepath.ToSlash(path)]
		if !found || d.Mtime != mtime {
			return DirectoryReuse{}, false
		}
		return d, true
	}, nil
}
