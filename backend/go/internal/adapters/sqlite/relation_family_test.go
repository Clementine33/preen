package sqlite_test

import (
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works"
)

// familyCodes: RJ00000001..RJ00000010, of which only the first six are catalog works.
func familyCode(n int) works.WorkCode { return works.WorkCode(fmt.Sprintf("RJ%08d", n)) }

const (
	familyCodeCount    = 10
	familyCatalogCount = 6
)

func familyRepository(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "family.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	for n := 1; n <= familyCatalogCount; n++ {
		if _, err := repo.DB().Exec(
			`INSERT INTO works(id,collected_at,updated_at) VALUES(?,?,?)`,
			string(familyCode(n)), deletionStamp, deletionStamp,
		); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func emptyFamilyEvidence() works.Evidence {
	return works.Evidence{
		Self:             map[works.WorkCode]works.RelationSelf{},
		DeclaredChildren: map[works.WorkCode][]works.WorkCode{},
	}
}

// familyReference reads the whole evidence the way the global path does.
func familyReference(t *testing.T, db *sql.DB) works.Evidence {
	t.Helper()
	ev := emptyFamilyEvidence()
	rows, err := db.Query("SELECT subject_rj, role, language, original_rj, parent_rj FROM relation_reports")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, role string
		var language, original, parent sql.NullString
		if scanErr := rows.Scan(&id, &role, &language, &original, &parent); scanErr != nil {
			t.Fatal(scanErr)
		}
		self := works.RelationSelf{Role: works.RelationRole(role)}
		if language.Valid {
			self.Language = &language.String
		}
		if original.Valid {
			self.OriginalRJ = new(works.WorkCode(original.String))
		}
		if parent.Valid {
			self.ParentRJ = new(works.WorkCode(parent.String))
		}
		ev.Self[works.WorkCode(id)] = self
	}
	_ = rows.Close()
	rows, err = db.Query(
		"SELECT declaring_rj, child_rj FROM relation_declared_children ORDER BY declaring_rj, child_rj",
	)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var declaring, child string
		if err := rows.Scan(&declaring, &child); err != nil {
			t.Fatal(err)
		}
		ev.DeclaredChildren[works.WorkCode(declaring)] = append(
			ev.DeclaredChildren[works.WorkCode(declaring)],
			works.WorkCode(child),
		)
	}
	_ = rows.Close()
	return ev
}

func familyTranslations(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(
		"SELECT work_id, role, language, original_work_id, parent_work_id FROM work_translations ORDER BY work_id",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		var role, language, original, parent sql.NullString
		if err := rows.Scan(&id, &role, &language, &original, &parent); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprint(id, role, language, original, parent))
	}
	return out
}

// familyExpect computes what the global path would do for one import: the
// refusal code ("" when accepted), the translation table afterwards and the
// warnings returned.
func familyExpect(
	t *testing.T, db *sql.DB, target works.WorkCode, group *works.RelationsGroup,
) (string, []string, []works.RelationWarning) {
	t.Helper()
	before := familyReference(t, db)
	after := emptyFamilyEvidence()
	maps.Copy(after.Self, before.Self)
	maps.Copy(after.DeclaredChildren, before.DeclaredChildren)
	delete(after.DeclaredChildren, target)
	after.Self[target] = works.RelationSelf{
		Role: group.Role, Language: group.Language, OriginalRJ: group.OriginalRJ, ParentRJ: group.ParentRJ,
	}
	if len(group.ChildRJs) > 0 {
		after.DeclaredChildren[target] = group.ChildRJs
	}
	resolution := works.Resolve(after)
	if violation := works.ValidateResolvedRelations(after, resolution); violation != nil {
		return violation.Code, familyTranslations(t, db), nil
	}
	affected := works.AffectedWorks(before, after, []works.WorkCode{target})
	// Apply the recomputation on a scratch copy of the table through the
	// same SQL the repository uses, by replaying it in a rolled-back tx.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, id := range affected {
		relation := resolution.Relations[id]
		if relation.IsEmpty() {
			_, err = tx.Exec("DELETE FROM work_translations WHERE work_id = ?", string(id))
		} else {
			var role, language, original, parent any
			if relation.Role != nil {
				role = string(*relation.Role)
			}
			if relation.Language != nil {
				language = *relation.Language
			}
			if relation.OriginalID != nil {
				original = string(*relation.OriginalID)
			}
			if relation.ParentID != nil {
				parent = string(*relation.ParentID)
			}
			_, err = tx.Exec(`INSERT INTO work_translations (work_id, role, language, original_work_id, parent_work_id)
				SELECT ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM works WHERE id = ?)
				ON CONFLICT(work_id) DO UPDATE SET role=excluded.role, language=excluded.language,
				original_work_id=excluded.original_work_id, parent_work_id=excluded.parent_work_id`,
				string(id), role, language, original, parent, string(id))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := tx.Query(
		"SELECT work_id, role, language, original_work_id, parent_work_id FROM work_translations ORDER BY work_id",
	)
	if err != nil {
		t.Fatal(err)
	}
	var table []string
	for rows.Next() {
		var id string
		var role, language, original, parent sql.NullString
		if err := rows.Scan(&id, &role, &language, &original, &parent); err != nil {
			t.Fatal(err)
		}
		table = append(table, fmt.Sprint(id, role, language, original, parent))
	}
	_ = rows.Close()
	var warnings []works.RelationWarning
	for _, warning := range resolution.Warnings {
		if slices.Contains(affected, warning.WorkID) {
			warnings = append(warnings, warning)
		}
	}
	return "", table, warnings
}

type familyClock struct{ n int }

func (c *familyClock) next() time.Time {
	c.n++
	return time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC).Add(time.Duration(c.n) * time.Second)
}

// familyStep imports one report and asserts the outcome equals the global path's.
func familyStep(
	t *testing.T, repo *sqlite.Repository, clock *familyClock, target works.WorkCode, group works.RelationsGroup,
) (refused string) {
	t.Helper()
	group.Source, group.FetchedAt = works.SourceDLsite, clock.next()
	wantCode, wantTable, wantWarnings := familyExpect(t, repo.DB(), target, &group)
	outcome, err := repo.ImportRelationReport(target, &group)
	var refusal *works.Error
	switch {
	case err == nil:
		if wantCode != "" {
			t.Fatalf("import %s %+v accepted, global path refuses with %s", target, group, wantCode)
		}
		if len(outcome.Applied) != 1 {
			t.Fatalf("import %s not applied: %+v", target, outcome)
		}
	case errors.As(err, &refusal):
		if refusal.Code != wantCode {
			t.Fatalf("import %s %+v refused with %s, global path says %q", target, group, refusal.Code, wantCode)
		}
	default:
		t.Fatal(err)
	}
	if got := familyTranslations(t, repo.DB()); !slices.Equal(got, wantTable) {
		t.Fatalf("import %s %+v\ntranslations = %v\nwant           %v", target, group, got, wantTable)
	}
	if err == nil && !reflect.DeepEqual(outcome.Warnings, wantWarnings) {
		t.Fatalf("import %s warnings = %+v, want %+v", target, outcome.Warnings, wantWarnings)
	}
	return wantCode
}

func familyGroup(role works.RelationRole, parent, original int, children ...int) works.RelationsGroup {
	group := works.RelationsGroup{Role: role, Language: new("ja")}
	if parent > 0 {
		group.ParentRJ = new(familyCode(parent))
	}
	if original > 0 {
		group.OriginalRJ = new(familyCode(original))
	}
	for _, child := range children {
		group.ChildRJs = append(group.ChildRJs, familyCode(child))
	}
	return group
}

func randomFamilyGroup(rng *rand.Rand) works.RelationsGroup {
	roles := []works.RelationRole{"original", "parent", "child"}
	pick := func(chance int) int {
		if rng.IntN(100) >= chance {
			return 0
		}
		return 1 + rng.IntN(familyCodeCount)
	}
	var children []int
	for range rng.IntN(4) {
		if c := 1 + rng.IntN(familyCodeCount); !slices.Contains(children, c) {
			children = append(children, c)
		}
	}
	return familyGroup(roles[rng.IntN(len(roles))], pick(40), pick(25), children...)
}

func TestRelationFamilyImportMatchesGlobalResolution(t *testing.T) {
	rng := rand.New(rand.NewPCG(33, 603))
	refusals := 0
	for iteration := range 300 {
		repo := familyRepository(t)
		clock := &familyClock{}
		// Build a globally valid state through accepted imports only.
		for range 4 + rng.IntN(8) {
			familyStep(t, repo, clock, familyCode(1+rng.IntN(familyCodeCount)), randomFamilyGroup(rng))
		}
		if v := works.ValidateRelations(familyReference(t, repo.DB())); v != nil {
			t.Fatalf("iteration %d: seeded state invalid: %+v", iteration, v)
		}
		if familyStep(t, repo, clock, familyCode(1+rng.IntN(familyCodeCount)), randomFamilyGroup(rng)) != "" {
			refusals++
		}
	}
	if refusals == 0 {
		t.Fatal("property run never exercised a refusal")
	}
}

func TestRelationFamilyRegressions(t *testing.T) {
	t.Run("deleted declaration splits a family and keeps the orphan's own relation", func(t *testing.T) {
		repo, clock := familyRepository(t), &familyClock{}
		familyStep(t, repo, clock, familyCode(2), familyGroup("original", 0, 0))
		familyStep(t, repo, clock, familyCode(1), familyGroup("parent", 0, 0, 2))
		before := familyTranslations(t, repo.DB())
		if len(before) == 0 {
			t.Fatal("no translation rows seeded")
		}
		familyStep(t, repo, clock, familyCode(1), familyGroup("parent", 0, 0))
		var role, language string
		if err := repo.DB().
			QueryRow("SELECT role, language FROM work_translations WHERE work_id = ?", string(familyCode(2))).
			Scan(&role, &language); err != nil || role != "original" ||
			language != "ja" {
			t.Fatalf("orphan row = %q %q, %v", role, language, err)
		}
	})
	t.Run("new edge merges two families", func(t *testing.T) {
		repo, clock := familyRepository(t), &familyClock{}
		familyStep(t, repo, clock, familyCode(1), familyGroup("child", 7, 0))
		familyStep(t, repo, clock, familyCode(2), familyGroup("original", 0, 0))
		familyStep(t, repo, clock, familyCode(3), familyGroup("parent", 0, 0, 4))
		familyStep(t, repo, clock, familyCode(7), familyGroup("parent", 0, 0, 3, 2))
		familyStep(t, repo, clock, familyCode(4), familyGroup("child", 1, 0))
	})
	t.Run("edges to external numbers", func(t *testing.T) {
		repo, clock := familyRepository(t), &familyClock{}
		familyStep(t, repo, clock, familyCode(1), familyGroup("child", 9, 10))
		familyStep(t, repo, clock, familyCode(9), familyGroup("parent", 0, 0, 8, 2))
		familyStep(t, repo, clock, familyCode(9), familyGroup("parent", 0, 0, 8))
	})
	t.Run("child declared by several works", func(t *testing.T) {
		repo, clock := familyRepository(t), &familyClock{}
		familyStep(t, repo, clock, familyCode(2), familyGroup("parent", 0, 0, 1))
		familyStep(t, repo, clock, familyCode(3), familyGroup("parent", 0, 0, 1))
		familyStep(t, repo, clock, familyCode(4), familyGroup("parent", 0, 0, 1, 3))
		familyStep(t, repo, clock, familyCode(2), familyGroup("parent", 0, 0))
	})
	t.Run("declaration cycle outranked by a work's own report", func(t *testing.T) {
		repo, clock := familyRepository(t), &familyClock{}
		familyStep(t, repo, clock, familyCode(1), familyGroup("parent", 0, 0, 2))
		if code := familyStep(t, repo, clock, familyCode(2), familyGroup("parent", 0, 0, 1)); code != "" {
			t.Fatalf("declaration cycle refused: %s", code)
		}
		familyStep(t, repo, clock, familyCode(2), familyGroup("original", 0, 0, 1))
	})
}

func TestRelationFamilyIgnoresViolationInAnotherFamily(t *testing.T) {
	repo, clock := familyRepository(t), &familyClock{}
	for _, row := range [][2]string{{"RJ00000008", "RJ00000009"}, {"RJ00000009", "RJ00000010"}} {
		if _, err := repo.DB().Exec(
			`INSERT INTO relation_reports(subject_rj, role, original_rj, source, fetched_at) VALUES(?, 'child', ?, 'dlsite', ?)`,
			row[0], row[1], deletionStamp,
		); err != nil {
			t.Fatal(err)
		}
	}
	if v := works.ValidateRelations(familyReference(t, repo.DB())); v == nil {
		t.Fatal("seeded violation is not a violation")
	}
	group := familyGroup("parent", 0, 0, 2)
	group.Source, group.FetchedAt = works.SourceDLsite, clock.next()
	if _, err := repo.ImportRelationReport(familyCode(1), &group); err != nil {
		t.Fatalf("valid import elsewhere refused: %v", err)
	}
}
