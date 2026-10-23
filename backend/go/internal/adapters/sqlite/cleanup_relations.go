package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"

	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/works"
)

// CleanupGroups orders finite candidate IDs by current report components.
// CleanupBatch rechecks excluded reports before deleting any component.
func (r *Repository) CleanupGroups(ctx context.Context, category cleanup.Category, ids []string) ([][]string, error) {
	if category != cleanup.RelationReports {
		groups := make([][]string, 0, len(ids))
		for i := range ids {
			groups = append(groups, ids[i:i+1:i+1])
		}
		return groups, ctx.Err()
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ev, err := loadEvidenceContext(ctx, tx)
	if err != nil {
		return nil, err
	}
	groups, err := works.PlanReportCleanup(ctx, ev, nil, nil)
	if err != nil {
		return nil, err
	}
	pending := map[string]bool{}
	for _, id := range ids {
		pending[id] = true
	}
	batches := [][]string{}
	for _, group := range groups {
		batch := []string{}
		for _, id := range group {
			if pending[string(id)] {
				batch = append(batch, string(id))
				delete(pending, string(id))
			}
		}
		if len(batch) > 0 {
			batches = append(batches, batch)
		}
	}
	// Missing reports still count as checked/skipped, without chasing new IDs.
	for _, id := range ids {
		if pending[id] {
			batches = append(batches, []string{id})
		}
	}
	return batches, ctx.Err()
}

func cleanupRelationGroups(
	ctx context.Context,
	tx *sql.Tx,
	selected []cleanup.Category,
	eligible []string,
) ([][]works.WorkCode, error) {
	roots, err := cleanupIDs(ctx, tx, "SELECT id FROM works ORDER BY id")
	if err != nil {
		return nil, err
	}
	if slices.Contains(selected, cleanup.Works) {
		removed, removeErr := cleanupWorks(ctx, tx)
		if removeErr != nil {
			return nil, removeErr
		}
		projected := map[string]bool{}
		for _, id := range removed {
			projected[id] = true
		}
		roots = slices.DeleteFunc(roots, func(id string) bool { return projected[id] })
	}
	ev, err := loadEvidenceContext(ctx, tx)
	if err != nil {
		return nil, err
	}
	return works.PlanReportCleanup(ctx, ev, cleanupWorkCodes(roots), cleanupWorkCodes(eligible))
}

func cleanupWorkCodes(ids []string) []works.WorkCode {
	if ids == nil {
		return nil
	}
	codes := make([]works.WorkCode, len(ids))
	for i, id := range ids {
		codes[i] = works.WorkCode(id)
	}
	return codes
}

func flattenCleanupGroups(groups [][]works.WorkCode) []string {
	ids := []string{}
	for _, group := range groups {
		for _, id := range group {
			ids = append(ids, string(id))
		}
	}
	return ids
}

func deleteCleanupReports(ctx context.Context, tx *sql.Tx, ids []string) (int64, error) {
	// All reports outside this batch are retained as roots. Their closure protects
	// new dependencies and prevents a current SCC from being partially deleted.
	groups, err := cleanupRelationGroups(ctx, tx, nil, ids)
	if err != nil {
		return 0, err
	}
	removable := flattenCleanupGroups(groups)
	if len(removable) == 0 {
		return 0, ctx.Err()
	}
	data, err := json.Marshal(removable)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(
		ctx,
		"DELETE FROM relation_reports WHERE subject_rj IN (SELECT value FROM json_each(?))",
		string(data),
	)
	if err != nil {
		return 0, err
	}
	ev, err := loadEvidenceContext(ctx, tx)
	if err != nil {
		return 0, err
	}
	if violation := works.ValidateRelations(ev); violation != nil {
		return 0, works.Invalid(violation.Code, violation.Message)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
