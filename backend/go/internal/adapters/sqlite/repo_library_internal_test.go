package sqlite

import (
	"strings"
	"testing"
)

func TestLibraryCountsSearchDirectoryPathRanges(t *testing.T) {
	repo := newTestRepository(t)
	for _, filter := range []string{"", " AND f.is_audio = 1"} {
		plan := explain(t, repo, `SELECT d.path, (SELECT COUNT(*) `+librarySubtreeFilesSQL+filter+`)
   FROM inventory_dirs d WHERE d.root_path = ? AND d.is_work_root = 1 ORDER BY d.path`)
		t.Log(plan)
		if !strings.Contains(plan, "root_path=? AND path>? AND path<?") ||
			!strings.Contains(plan, "SEARCH f USING PRIMARY KEY") || strings.Contains(plan, "SCAN f") {
			t.Fatalf("library counts must visit only each work subtree: %s", plan)
		}
	}
}
