package coverdelivery

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Recover is called before workers start; stopped requests remain stopped.
func (s *Service) Recover(ctx context.Context) error {
	if err := s.deps.Store.RecoverCovers(ctx); err != nil {
		return err
	}
	active, err := s.deps.Store.ActiveCoverJobs(ctx)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(s.deps.StageDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		id := strings.TrimSuffix(strings.TrimSuffix(name, ".tmp"), ".image")
		if _, err = uuid.Parse(
			id,
		); err != nil || (name != id+".image" && name != id+".image.tmp") || active[id] ||
			entry.IsDir() {
			continue
		}
		if err = os.Remove(filepath.Join(s.deps.StageDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
