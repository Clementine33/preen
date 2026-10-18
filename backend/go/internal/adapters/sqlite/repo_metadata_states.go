package sqlite

import (
	"context"
	"database/sql"

	"github.com/Clementine33/preen/backend/internal/works"
)

// MetadataStates reads group presence in one catalog snapshot, without adding crawler state.
func (r *Repository) MetadataStates(
	ctx context.Context,
	ids []works.WorkCode,
) (map[works.WorkCode]map[works.DataGroup]works.SourceState, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := make(map[works.WorkCode]map[works.DataGroup]works.SourceState, len(ids))
	for _, id := range ids {
		states, err := loadSourceStates(tx, id)
		if err != nil {
			return nil, err
		}
		out[id] = states
	}
	return out, tx.Commit()
}
