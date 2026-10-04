package filesystem

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/onsei/organizer/backend/internal/works/coverdelivery"
)

// CoverStaging owns download-file naming, bounded reads, atomic replacement and
// cleanup. The workflow supplies active job identities from its durable store.
type CoverStaging struct{ dir string }

func NewCoverStaging(dir string) (*CoverStaging, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &CoverStaging{dir: dir}, nil
}

func (s *CoverStaging) path(jobID string) string { return filepath.Join(s.dir, jobID+".image") }

func (s *CoverStaging) Read(jobID, savedPath string) ([]byte, error) {
	if savedPath != s.path(jobID) {
		return nil, fs.ErrNotExist
	}
	f, err := os.Open(savedPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, coverdelivery.MaxBytes+1))
}

func (s *CoverStaging) Write(jobID string, data []byte) (string, error) {
	path := s.path(jobID)
	temp := path + ".tmp"
	defer func() { _ = os.Remove(temp) }()
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *CoverStaging) Remove(jobID, savedPath string) error {
	if savedPath == s.path(jobID) {
		if err := os.Remove(savedPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *CoverStaging) Cleanup(activeJobs map[string]bool) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		id := strings.TrimSuffix(strings.TrimSuffix(name, ".tmp"), ".image")
		if _, err = uuid.Parse(id); err != nil ||
			(name != id+".image" && name != id+".image.tmp") || activeJobs[id] || entry.IsDir() {
			continue
		}
		if err = os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
