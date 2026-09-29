package app

import (
	"path/filepath"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
)

// RebuildDevelopmentDatabase rebuilds only .dev_data, resolving the repository
// root from the backend/go working directory used by task db:rebuild:dev.
func RebuildDevelopmentDatabase() (string, error) {
	path, err := filepath.Abs(filepath.Join("..", "..", ".dev_data", "cache.db"))
	if err != nil {
		return "", err
	}
	return sqlite.RebuildDevelopmentDatabase(path)
}
