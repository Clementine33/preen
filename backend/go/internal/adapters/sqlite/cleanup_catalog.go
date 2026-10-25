package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/works"
)

func cleanupIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func cleanupWorks(ctx context.Context, tx *sql.Tx) ([]string, error) {
	ids, err := cleanupIDs(ctx, tx, "SELECT id FROM works ORDER BY id")
	if err != nil {
		return nil, err
	}
	eligible := []string{}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		blockers, err := collectBlockers(tx, works.WorkCode(id))
		if err != nil {
			return nil, err
		}
		if len(blockers) == 0 {
			eligible = append(eligible, id)
		}
	}
	return eligible, nil
}

// References use the same j.work_id alias so projection can release only
// selected eligible works without mutating the preview's snapshot.
func cleanupSource(category cleanup.Category) (string, string) {
	switch category {
	case cleanup.SourceTags:
		return "tags", "SELECT 1 FROM work_tags j WHERE j.tag_id = tags.id"
	case cleanup.SourceVoiceActors:
		return "voice_actors", "SELECT 1 FROM work_voice_actors j WHERE j.voice_actor_id = voice_actors.id"
	case cleanup.SourceCircles:
		return "circles", "SELECT 1 FROM (SELECT id AS work_id, circle_id FROM works) j WHERE j.circle_id = circles.id"
	case cleanup.ScanSessions,
		cleanup.PlanGenerations,
		cleanup.CoverDownloads,
		cleanup.GenerationRecords,
		cleanup.Works,
		cleanup.RelationReports:
		return "", ""
	default:
		return "", ""
	}
}

func deleteCleanupWorks(ctx context.Context, tx *sql.Tx, ids []string) (int64, error) {
	var deleted int64
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		blockers, err := collectBlockers(tx, works.WorkCode(id))
		if err != nil {
			return 0, err
		}
		if len(blockers) > 0 {
			continue
		}
		result, err := tx.ExecContext(ctx, "DELETE FROM works WHERE id = ?", id)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += n
	}
	return deleted, nil
}

func deleteCleanupSources(ctx context.Context, tx *sql.Tx, category cleanup.Category, ids []string) (int64, error) {
	table, reference := cleanupSource(category)
	data, err := json.Marshal(ids)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(
		ctx,
		"DELETE FROM "+table+" WHERE work_id IS NULL AND id IN (SELECT value FROM json_each(?)) AND NOT EXISTS ("+reference+")",
		string(data),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
