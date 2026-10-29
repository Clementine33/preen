package sqlite

import (
	"context"

	"github.com/Clementine33/preen/backend/internal/dirrename"
)

// InterruptedRenames retains frozen requests for repair without resuming them.
func (r *Repository) InterruptedRenames(ctx context.Context) ([]dirrename.InterruptedExecution, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT e.execution_id,w.root_path,e.request_json FROM plan_executions e JOIN worksets w ON w.id=e.workset_id
		WHERE e.operation_type='rename' AND e.status='interrupted'
		AND e.plan_id IN (SELECT plan_id FROM plans WHERE status <> 'retired') ORDER BY e.created_at,e.execution_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dirrename.InterruptedExecution
	for rows.Next() {
		var ex dirrename.InterruptedExecution
		if err := rows.Scan(&ex.ID, &ex.Root, &ex.RequestJSON); err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}
