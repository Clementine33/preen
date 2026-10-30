package sqlite

import (
	"database/sql"
	"encoding/json"
	"slices"

	"github.com/Clementine33/preen/backend/internal/works"
)

// relationFamily is the evidence of every work reachable from some seeds over
// parent, original and declared-child edges in both directions, with the set of
// works it covers. The relation rules only follow those edges, so resolving and
// validating a family gives the same answer for its members as the whole table.
type relationFamily struct {
	evidence works.Evidence
	members  map[works.WorkCode]struct{}
}

// loadRelationFamily walks the family breadth-first, one indexed query per edge
// kind and frontier. Seeds and edge targets need no catalog row or report.
func loadRelationFamily(tx *sql.Tx, seeds []works.WorkCode) (relationFamily, error) {
	family := relationFamily{
		evidence: works.Evidence{
			Self:             map[works.WorkCode]works.RelationSelf{},
			DeclaredChildren: map[works.WorkCode][]works.WorkCode{},
		},
		members: map[works.WorkCode]struct{}{},
	}
	var frontier []works.WorkCode
	visit := func(id string) {
		code := works.WorkCode(id)
		if _, seen := family.members[code]; !seen {
			family.members[code] = struct{}{}
			frontier = append(frontier, code)
		}
	}
	for _, seed := range seeds {
		visit(string(seed))
	}
	for len(frontier) > 0 {
		ids := make([]string, len(frontier))
		for i, id := range frontier {
			ids[i] = string(id)
		}
		slices.Sort(ids)
		encoded, err := json.Marshal(ids)
		if err != nil {
			return family, err
		}
		frontier = nil
		if err := family.expand(tx, string(encoded), visit); err != nil {
			return family, err
		}
	}
	return family, nil
}

// expand loads the frontier's own reports and declarations and visits every
// neighbour: forward through the reports and declarations just read, backward
// through the reports and declarations that name a frontier work.
func (f relationFamily) expand(tx *sql.Tx, frontier string, visit func(string)) error {
	const in = " IN (SELECT value FROM json_each(?))"
	if err := scanRows(
		tx,
		"SELECT subject_rj, role, language, original_rj, parent_rj FROM relation_reports WHERE subject_rj"+in,
		frontier,
		func(rows *sql.Rows) error {
			var (
				id, role           string
				language, original sql.NullString
				parent             sql.NullString
			)
			if err := rows.Scan(&id, &role, &language, &original, &parent); err != nil {
				return err
			}
			f.evidence.Self[works.WorkCode(id)] = works.RelationSelf{
				Role:       works.RelationRole(role),
				Language:   nullString(language),
				OriginalRJ: nullWorkCode(original),
				ParentRJ:   nullWorkCode(parent),
			}
			for _, edge := range []sql.NullString{original, parent} {
				if edge.Valid {
					visit(edge.String)
				}
			}
			return nil
		},
	); err != nil {
		return err
	}
	if err := scanRows(
		tx,
		"SELECT declaring_rj, child_rj FROM relation_declared_children WHERE declaring_rj"+in,
		frontier,
		func(rows *sql.Rows) error {
			var declaring, child string
			if err := rows.Scan(&declaring, &child); err != nil {
				return err
			}
			f.evidence.DeclaredChildren[works.WorkCode(declaring)] = append(
				f.evidence.DeclaredChildren[works.WorkCode(declaring)], works.WorkCode(child),
			)
			visit(child)
			return nil
		},
	); err != nil {
		return err
	}
	// Reverse edges: only the neighbour's identity matters here; its own
	// report and declarations are read when it is the frontier.
	for _, query := range []string{
		"SELECT subject_rj FROM relation_reports WHERE parent_rj" + in,
		"SELECT subject_rj FROM relation_reports WHERE original_rj" + in,
		"SELECT declaring_rj FROM relation_declared_children WHERE child_rj" + in,
	} {
		if err := scanRows(tx, query, frontier, func(rows *sql.Rows) error {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			visit(id)
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func scanRows(tx *sql.Tx, query, arg string, each func(*sql.Rows) error) error {
	rows, err := tx.Query(query, arg)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := each(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
