package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/onsei/organizer/backend/internal/pathnorm"
	"github.com/onsei/organizer/backend/internal/rename"
)

// RenameWorks reads catalog bindings and naming fields in a single query snapshot.
func (r *Repository) RenameWorks(ctx context.Context, root string, folders []string) (map[string]rename.Work, error) {
	out := make(map[string]rename.Work, len(folders))
	if len(folders) == 0 {
		return out, nil
	}
	args := []any{pathnorm.RootPathKey(root)}
	marks := make([]string, len(folders))
	for i, folder := range folders {
		marks[i] = "?"
		args = append(args, filepath.Base(filepath.FromSlash(folder)))
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT wc.rel_path,w.id,COALESCE(w.title,''),COALESCE(c.name,''),COALESCE(w.release_date,''),COALESCE(tr.language,''),
 (SELECT json_group_array(va.name) FROM work_voice_actors wva JOIN voice_actors va ON va.id=wva.voice_actor_id WHERE wva.work_id=w.id)
 FROM work_copies wc JOIN libraries l ON l.id=wc.library_id JOIN works w ON w.id=wc.work_id
 LEFT JOIN circles c ON c.id=w.circle_id LEFT JOIN work_translations tr ON tr.work_id=w.id
 WHERE l.root_path_key=? AND wc.rel_path IN (`+strings.Join(
			marks,
			",",
		)+`)`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rel, code, title, circle, date, language, actorsJSON string
		if err := rows.Scan(&rel, &code, &title, &circle, &date, &language, &actorsJSON); err != nil {
			return nil, err
		}
		actors := []string{}
		if err := json.Unmarshal([]byte(actorsJSON), &actors); err != nil {
			return nil, err
		}
		slices.Sort(actors)
		actors = slices.Compact(actors)
		out[filepath.ToSlash(filepath.Join(root, rel))] = rename.Work{
			Code: code,
			Values: map[string]string{
				"title":        title,
				"circle":       circle,
				"release_date": date,
				"language":     language,
				"voice_actors": strings.Join(actors, "、"),
			},
		}
	}
	return out, rows.Err()
}
