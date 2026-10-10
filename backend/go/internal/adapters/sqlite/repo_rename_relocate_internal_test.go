package sqlite

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/librarypath"
)

// explain returns the plan text of a statement run with placeholder arguments.
func explain(t *testing.T, r *Repository, statement string) string {
	t.Helper()
	args := make([]any, strings.Count(statement, "?"))
	for i := range args {
		args[i] = "x"
	}
	rows, err := r.db.Query("EXPLAIN QUERY PLAN "+statement, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	return strings.Join(plan, "\n")
}

// A whole-library rename relocates once per folder, so each statement has to
// search by path range. A plan that only says SEARCH (library_id=?) would still
// visit every row of the library, hence the exact range text.
func TestRelocationStatementsSearchByPathRange(t *testing.T) {
	repo, err := NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	for _, tc := range []struct {
		statement string
		want      string
	}{
		{relocateEntriesSQL, "path>? AND path<?"},
		{relocateCredentialsSQL, "path>? AND path<?"},
		{selectCopiesSQL, "library_id=? AND rel_path>? AND rel_path<?"},
		{selectMembersSQL, "workset_id=? AND rel_path>? AND rel_path<?"},
	} {
		plan := explain(t, repo, tc.statement)
		if !strings.Contains(plan, tc.want) || strings.Contains(plan, "SCAN entries") ||
			strings.Contains(plan, "SCAN generation_records") {
			t.Errorf("%.40s…\nwant %q in plan:\n%s", tc.statement, tc.want, plan)
		}
	}
}

func TestRelocateDirectoryAcceptsARenameBackAndRejectsOtherShapes(t *testing.T) {
	repo, err := NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.RelocateDirectoryAs(t.Context(), "/lib", "/lib/a", "/lib/a", ""); err != nil {
		t.Fatalf("renamed back to the recorded name: %v", err)
	}
	for _, bad := range [][2]string{{"/lib", "/lib"}, {"/lib/a", "/lib/b/c"}, {"/elsewhere/a", "/elsewhere/b"}} {
		if err := repo.RelocateDirectoryAs(t.Context(), "/lib", bad[0], bad[1], ""); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

func TestSelectedPathsCoverThemselvesAndTheirDescendantsOnly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selected []string
		key      string
		want     bool
	}{
		{"equal", []string{"/lib/a"}, "/lib/a", true},
		{"descendant", []string{"/lib/a"}, "/lib/a/b/c", true},
		{"shared prefix is a sibling", []string{"/lib/a"}, "/lib/ab", false},
		{"parent is not covered", []string{"/lib/a/b"}, "/lib/a", false},
		{"UNC descendant", []string{"//server/share/work"}, "//server/share/work/disc", true},
		{"UNC sibling", []string{"//server/share/work"}, "//server/share/work2/disc", false},
		{"UNC share root", []string{"//server/share"}, "//server/share/work/disc", true},
		{"drive root", []string{"C:/"}, "c:/lib/x", true},
		{"drive path is case-folded", []string{"C:/Lib"}, "c:/lib/x/y", true},
		{"filesystem root", []string{"/"}, "/lib/a", true},
		{"other drive", []string{"C:/lib"}, "d:/lib/x", false},
	} {
		if got := newSelectedPaths(tc.selected).covers(librarypath.RootPathKey(tc.key)); got != tc.want {
			t.Errorf("%s: covers(%q) = %v, want %v", tc.name, tc.key, got, tc.want)
		}
	}
}

func TestRelocateDirectoryMovesOnlyTheSubtree(t *testing.T) {
	repo, err := NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	for _, p := range []string{"/lib/a", "/lib/a/f", "/lib/ab", "/lib/ab/f", "/lib/a_", "/lib/a%"} {
		if _, execErr := repo.db.Exec(
			`INSERT INTO entries(path,name,parent_path,root_path) VALUES(?,?,?,?)`,
			p,
			filepath.Base(p),
			filepath.Dir(p),
			"/lib",
		); execErr != nil {
			t.Fatal(execErr)
		}
	}
	if relocateErr := repo.RelocateDirectory(t.Context(), "/lib", "/lib/a", "/lib/c"); relocateErr != nil {
		t.Fatal(relocateErr)
	}
	rows, err := repo.db.Query(`SELECT path FROM entries ORDER BY path`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		got = append(got, p)
	}
	if want := "/lib/a%,/lib/a_,/lib/ab,/lib/ab/f,/lib/c,/lib/c/f"; strings.Join(got, ",") != want {
		t.Fatalf("paths = %v, want %s", got, want)
	}
}
