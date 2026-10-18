package sqlite

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Clementine33/preen/backend/internal/librarypath"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

// HasActiveGenerationForRoot includes queued planning sessions of every kind.
func (r *Repository) HasActiveGenerationForRoot(root string) (bool, error) {
	var active bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM plan_generations g JOIN worksets w ON w.id=g.workset_id WHERE w.root_path_key=? AND g.status IN ('queued','running'))`, librarypath.RootPathKey(root)).
		Scan(&active)
	return active, err
}

// HasActiveExecutionForRootKinds supplies the task-declared exclusion policy.
func (r *Repository) HasActiveExecutionForRootKinds(root string, kinds []string) (bool, error) {
	if len(kinds) == 0 {
		return false, nil
	}
	args := []any{librarypath.RootPathKey(root)}
	marks := make([]string, len(kinds))
	for i, k := range kinds {
		marks[i] = "?"
		args = append(args, k)
	}
	var active bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM plan_executions e JOIN worksets w ON w.id=e.workset_id WHERE w.root_path_key=? AND e.status IN ('queued','running') AND e.operation_type IN (`+strings.Join(marks, ",")+`))`, args...).
		Scan(&active)
	return active, err
}

// HasActiveRenameExecutionForRoot is the directory-mutation check for page refreshes.
func (r *Repository) HasActiveRenameExecutionForRoot(root string) (bool, error) {
	return r.HasActiveExecutionForRootKinds(root, []string{"rename"})
}

// HasPendingRenameCovers checks frozen delivery paths, including a running
// shared download whose delivery remains pending. Terminal failures do not block.
func (r *Repository) HasPendingRenameCovers(ctx context.Context, root string, paths []string) (bool, error) {
	if len(paths) == 0 {
		return false, nil
	}
	selected := newSelectedPaths(paths)
	rootKey := librarypath.RootPathKey(root)
	rows, err := r.db.QueryContext(ctx, `SELECT target_json FROM cover_deliveries WHERE status='pending'`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		var t coverdelivery.Target
		if err := json.Unmarshal([]byte(raw), &t); err != nil {
			return false, err
		}
		if librarypath.RootPathKey(t.LibraryRoot) != rootKey {
			continue
		}
		if selected.covers(librarypath.RootPathKey(librarypath.JoinRel(root, t.RelPath))) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// selectedPaths answers whether a path key is one of the selected paths or
// lies below one. A key is cut at its own "/" characters, never with path.Dir,
// so "//server/share", "C:/" and "/" keep their meaning.
type selectedPaths struct{ full, trimmed map[string]struct{} }

func newSelectedPaths(paths []string) selectedPaths {
	s := selectedPaths{full: make(map[string]struct{}, len(paths)), trimmed: make(map[string]struct{}, len(paths))}
	for _, selected := range paths {
		key := librarypath.RootPathKey(selected)
		s.full[key] = struct{}{}
		s.trimmed[strings.TrimSuffix(key, "/")] = struct{}{}
	}
	return s
}

func (s selectedPaths) covers(key string) bool {
	if _, ok := s.full[key]; ok {
		return true
	}
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] != '/' {
			continue
		}
		if _, ok := s.trimmed[key[:i]]; ok {
			return true
		}
	}
	return false
}
