package sqlite

import (
	"context"
	"database/sql"

	"github.com/Clementine33/preen/backend/internal/works"
)

// ImportRelationReport atomically accepts external evidence, validates the
// resulting graph and updates dependent catalog works, without creating works.
func (r *Repository) ImportRelationReport(
	target works.WorkCode,
	group *works.RelationsGroup,
) (works.ImportOutcome, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return works.ImportOutcome{}, err
	}
	defer tx.Rollback()
	states, err := loadSourceStates(tx, target)
	if err != nil {
		return works.ImportOutcome{}, err
	}
	outcome := works.ImportOutcome{}
	warnings, err := applyRelationsGroup(tx, target, group, states, &outcome)
	if err != nil {
		return works.ImportOutcome{}, err
	}
	outcome.Warnings = warnings
	if err := tx.Commit(); err != nil {
		return works.ImportOutcome{}, err
	}
	r.catalogChanged()
	return outcome, nil
}

// applyRelationsGroup replaces the subject's independent report and the child
// edges it declared, then recomputes every derived relation the change touches.
//
// The report is a snapshot, so a submission replaces both rows wholesale; a
// group that is not submitted leaves both untouched.
func applyRelationsGroup(
	tx *sql.Tx,
	target works.WorkCode,
	group *works.RelationsGroup,
	stored map[works.DataGroup]works.SourceState,
	outcome *works.ImportOutcome,
) ([]works.RelationWarning, error) {
	state, present := stored[works.GroupRelations]
	decision := works.DecideGroup(works.GroupRelations, group.Source, group.FetchedAt.UnixNano(), state, present)
	if !recordGroup(outcome, works.GroupRelations, decision, works.AppliedGroup{DataGroup: works.GroupRelations}) {
		return nil, nil
	}

	before, err := loadEvidence(tx)
	if err != nil {
		return nil, err
	}

	if _, execErr := tx.Exec("DELETE FROM relation_reports WHERE subject_rj = ?", string(target)); execErr != nil {
		return nil, execErr
	}

	if _, execErr := tx.Exec(`
		INSERT INTO relation_reports (subject_rj, role, language, original_rj, parent_rj, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, string(target), string(group.Role), nullableString(group.Language),
		nullableCode(group.OriginalRJ), nullableCode(group.ParentRJ),
		string(group.Source), formatWorksTimestamp(group.FetchedAt),
	); execErr != nil {
		return nil, execErr
	}
	for _, child := range group.ChildRJs {
		if _, execErr := tx.Exec(`
			INSERT INTO relation_declared_children (declaring_rj, child_rj)
			VALUES (?, ?)
		`, string(target), string(child)); execErr != nil {
			return nil, execErr
		}
	}

	after, err := loadEvidence(tx)
	if err != nil {
		return nil, err
	}
	// The structural rules run against the catalog this submission leaves
	// behind, so a relation that only becomes illegal in combination with the
	// stored evidence is still refused - and the whole submission goes with it.
	resolution := works.Resolve(after)
	if violation := works.ValidateResolvedRelations(after, resolution); violation != nil {
		return nil, works.Invalid(violation.Code, violation.Message)
	}

	affected := works.AffectedWorks(before, after, []works.WorkCode{target})
	if err := recomputeRelations(tx, resolution, affected); err != nil {
		return nil, err
	}
	// Only the warnings this submission is responsible for: the ones touching
	// the work it named or a work the change reached.
	return warningsFor(resolution.Warnings, affected), nil
}

// warningsFor keeps the warnings about the works a change reached.
func warningsFor(warnings []works.RelationWarning, affected []works.WorkCode) []works.RelationWarning {
	inScope := make(map[works.WorkCode]struct{}, len(affected))
	for _, id := range affected {
		inScope[id] = struct{}{}
	}
	var kept []works.RelationWarning
	for _, warning := range warnings {
		if _, ok := inScope[warning.WorkID]; ok {
			kept = append(kept, warning)
		}
	}
	return kept
}

// loadEvidence reads both evidence tables: what works said about themselves and
// what child edges they declared.
func loadEvidence(tx *sql.Tx) (works.Evidence, error) {
	return loadEvidenceContext(context.Background(), tx)
}

func loadEvidenceContext(ctx context.Context, tx *sql.Tx) (works.Evidence, error) {
	evidence := works.Evidence{
		Self:             map[works.WorkCode]works.RelationSelf{},
		DeclaredChildren: map[works.WorkCode][]works.WorkCode{},
	}

	selfRows, queryErr := tx.QueryContext(
		ctx,
		"SELECT subject_rj, role, language, original_rj, parent_rj FROM relation_reports",
	)
	if queryErr != nil {
		return evidence, queryErr
	}
	if rowsErr := func() error {
		defer selfRows.Close()
		for selfRows.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var (
				id, role           string
				language, original sql.NullString
				parent             sql.NullString
			)
			if scanErr := selfRows.Scan(&id, &role, &language, &original, &parent); scanErr != nil {
				return scanErr
			}
			evidence.Self[works.WorkCode(id)] = works.RelationSelf{
				Role:       works.RelationRole(role),
				Language:   nullString(language),
				OriginalRJ: nullWorkCode(original),
				ParentRJ:   nullWorkCode(parent),
			}
		}
		return selfRows.Err()
	}(); rowsErr != nil {
		return evidence, rowsErr
	}

	childRows, err := tx.QueryContext(ctx, "SELECT declaring_rj, child_rj FROM relation_declared_children")
	if err != nil {
		return evidence, err
	}
	defer childRows.Close()
	for childRows.Next() {
		if err := ctx.Err(); err != nil {
			return evidence, err
		}
		var declaring, child string
		if err := childRows.Scan(&declaring, &child); err != nil {
			return evidence, err
		}
		evidence.DeclaredChildren[works.WorkCode(declaring)] = append(
			evidence.DeclaredChildren[works.WorkCode(declaring)], works.WorkCode(child),
		)
	}
	return evidence, childRows.Err()
}

// recomputeRelations rewrites the derived relation of every affected work from
// the evidence, deleting a row whose resolution says nothing at all - a work
// that gave up its parent has no derived values left to keep.
func recomputeRelations(tx *sql.Tx, resolution works.Resolution, affected []works.WorkCode) error {
	for _, id := range affected {
		relation := resolution.Relations[id]
		if relation.IsEmpty() {
			if _, err := tx.Exec("DELETE FROM work_translations WHERE work_id = ?", string(id)); err != nil {
				return err
			}
			continue
		}
		var role any
		if relation.Role != nil {
			role = string(*relation.Role)
		}
		if _, err := tx.Exec(`
			INSERT INTO work_translations (work_id, role, language, original_work_id, parent_work_id)
			SELECT ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM works WHERE id = ?)
			ON CONFLICT(work_id) DO UPDATE SET
				role = excluded.role,
				language = excluded.language,
				original_work_id = excluded.original_work_id,
				parent_work_id = excluded.parent_work_id
		`, string(id), role, nullableString(relation.Language),
			nullableCode(relation.OriginalID), nullableCode(relation.ParentID), string(id),
		); err != nil {
			return err
		}
	}
	return nil
}

// RelationParent resolves the effective parent of a canonical number, including external reports.
func (r *Repository) RelationParent(ctx context.Context, id works.WorkCode) (*works.WorkCode, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	evidence, err := loadEvidence(tx)
	if err != nil {
		return nil, err
	}
	return works.Resolve(evidence).Relations[id].ParentID, nil
}
