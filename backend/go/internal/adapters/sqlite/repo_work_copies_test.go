package sqlite //nolint:testpackage // white-box tests drive the merge against the real schema

import (
	"fmt"
	"slices"
	"testing"

	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/works"
)

// stageDir inserts one direct-child directory row into a scan's staging set.
func stageDir(t *testing.T, repo *Repository, session, root, name string) {
	t.Helper()
	if _, err := repo.DB().Exec(`
		INSERT INTO entries_staging (session_id, path, root_path, parent_path, name, is_dir, is_work_root, size, mtime, operation)
		VALUES (?, ?, ?, ?, ?, 1, ?, 0, 0, 'upsert')
	`, session, root+"/"+name, root, root, name, works.CodesInDirName(name).Kind == works.DirCodeSingle); err != nil {
		t.Fatalf("stage %s: %v", name, err)
	}
}

// TestScanBindsDirectChildDirectories is the binding rule end to end: only a
// directory naming exactly one work is bound, the recovery directory is not a
// member, and the unclear names are reported instead of guessed at.
func TestScanBindsDirectChildDirectories(t *testing.T) {
	repo := newTestRepository(t)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}

	const root = "/music"
	for _, name := range []string{
		"RJ00000001 album title",
		"a directory with no number",
		"RJ00000002 - RJ00000003",
		"Delete",
	} {
		stageDir(t, repo, "scan-1", root, name)
	}
	outcome, err := repo.MergeStaging("scan-1", library.ID, root)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if outcome.DirsWithMultipleNumbers != 1 {
		t.Errorf("dirs_with_multiple_numbers = %d, want 1", outcome.DirsWithMultipleNumbers)
	}
	// The recovery directory is not a member, so it is not reported as unclear
	// either.
	for _, example := range outcome.ExampleDirs {
		if example == "Delete" {
			t.Error("the recovery directory was reported as an unclear member name")
		}
	}

	assertBindings(t, repo, library.ID, []binding{
		{relPath: "RJ00000001 album title", workID: "RJ00000001"},
	})
}

type binding struct {
	relPath string
	workID  string
}

func assertBindings(t *testing.T, repo *Repository, libraryID string, want []binding) {
	t.Helper()
	rows, err := repo.DB().Query(
		"SELECT rel_path, work_id FROM work_copies WHERE library_id = ? ORDER BY rel_path", libraryID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []binding
	for rows.Next() {
		var item binding
		if err := rows.Scan(&item.relPath, &item.workID); err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("bindings = %+v, want %+v", got, want)
	}
}

// TestRebindingKeepsTheWorksIdentity: renaming a directory away from its work
// unbinds it, and renaming it back rebinds the same work without resetting when
// it was first collected.
func TestRebindingKeepsTheWorksIdentity(t *testing.T) {
	repo := newTestRepository(t)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	const root = "/music"

	scan := func(name string) {
		t.Helper()
		stageDir(t, repo, "scan", root, name)
		if _, mergeErr := repo.MergeStaging("scan", library.ID, root); mergeErr != nil {
			t.Fatalf("merge: %v", mergeErr)
		}
	}

	scan("RJ00000001 album")
	assertBindings(t, repo, library.ID, []binding{{relPath: "RJ00000001 album", workID: "RJ00000001"}})

	var collected string
	if err := repo.DB().QueryRow(
		"SELECT collected_at FROM works WHERE id = 'RJ00000001'",
	).Scan(&collected); err != nil {
		t.Fatal(err)
	}

	// The directory stops naming a work at all: the binding goes, and the work
	// and its collection time stay behind.
	scan("album without a number")
	assertBindings(t, repo, library.ID, nil)
	var afterUnbind string
	if err := repo.DB().QueryRow(
		"SELECT collected_at FROM works WHERE id = 'RJ00000001'",
	).Scan(&afterUnbind); err != nil {
		t.Fatal(err)
	}
	if afterUnbind != collected {
		t.Errorf("unbinding reset collected_at: %q -> %q", collected, afterUnbind)
	}

	// Renaming it back rebinds the same work and still does not reset the time.
	scan("RJ00000001 album")
	assertBindings(t, repo, library.ID, []binding{{relPath: "RJ00000001 album", workID: "RJ00000001"}})
	var afterRebind string
	if err := repo.DB().QueryRow(
		"SELECT collected_at FROM works WHERE id = 'RJ00000001'",
	).Scan(&afterRebind); err != nil {
		t.Fatal(err)
	}
	if afterRebind != collected {
		t.Errorf("rebinding reset collected_at: %q -> %q", collected, afterRebind)
	}
}

// TestRescanningKeepsTheCopyIdentity: the copy id belongs to the library and
// the path, so re-scanning the same directory keeps the row it already had
// instead of replacing it.
func TestRescanningKeepsTheCopyIdentity(t *testing.T) {
	repo := newTestRepository(t)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	const root = "/music"

	stageDir(t, repo, "scan-a", root, "RJ00000001 album")
	if _, err := repo.MergeStaging("scan-a", library.ID, root); err != nil {
		t.Fatal(err)
	}
	var firstID string
	if err := repo.DB().QueryRow(
		"SELECT id FROM work_copies WHERE library_id = ?", library.ID,
	).Scan(&firstID); err != nil {
		t.Fatal(err)
	}

	// The next scan sees the same library and the same path, so the binding is
	// updated in place rather than dropped and made again.
	stageDir(t, repo, "scan-b", root, "RJ00000001 album")
	if _, err := repo.MergeStaging("scan-b", library.ID, root); err != nil {
		t.Fatal(err)
	}
	var secondID, workID string
	if err := repo.DB().QueryRow(
		"SELECT id, work_id FROM work_copies WHERE library_id = ?", library.ID,
	).Scan(&secondID, &workID); err != nil {
		t.Fatal(err)
	}
	if secondID != firstID {
		t.Errorf("re-scanning replaced the binding: %s -> %s", firstID, secondID)
	}
	if workID != "RJ00000001" {
		t.Errorf("work = %s, want the same directory's work", workID)
	}
}

// TestMemberRefreshLeavesBindingsAlone: a scoped scan reconciles nothing, so
// another directory's binding survives it.
func TestMemberRefreshLeavesBindingsAlone(t *testing.T) {
	repo := newTestRepository(t)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	const root = "/music"

	stageDir(t, repo, "scan-full", root, "RJ00000001 album")
	stageDir(t, repo, "scan-full", root, "RJ00000002 album")
	if _, err := repo.MergeStaging("scan-full", library.ID, root); err != nil {
		t.Fatal(err)
	}
	assertBindings(t, repo, library.ID, []binding{
		{relPath: "RJ00000001 album", workID: "RJ00000001"},
		{relPath: "RJ00000002 album", workID: "RJ00000002"},
	})

	// A member refresh names no library, which is how the pipeline says "do not
	// reconcile".
	if _, err := repo.MergeStaging("scan-member", "", root+"/RJ00000001 album"); err != nil {
		t.Fatal(err)
	}
	assertBindings(t, repo, library.ID, []binding{
		{relPath: "RJ00000001 album", workID: "RJ00000001"},
		{relPath: "RJ00000002 album", workID: "RJ00000002"},
	})
}

// TestRootChangeUnbindsTheLibrary: the bindings name directories under the old
// root, so a root change drops them while the works survive.
func TestRootChangeUnbindsTheLibrary(t *testing.T) {
	repo := newTestRepository(t)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	stageDir(t, repo, "scan", "/music", "RJ00000001 album")
	if _, err := repo.MergeStaging("scan", library.ID, "/music"); err != nil {
		t.Fatal(err)
	}
	assertBindings(t, repo, library.ID, []binding{{relPath: "RJ00000001 album", workID: "RJ00000001"}})

	if _, err := repo.UpdateLibrary(library.ID, "music", "/elsewhere"); err != nil {
		t.Fatal(err)
	}
	assertBindings(t, repo, library.ID, nil)

	var works int
	if err := repo.DB().QueryRow("SELECT COUNT(*) FROM works").Scan(&works); err != nil {
		t.Fatal(err)
	}
	if works != 1 {
		t.Errorf("works = %d, want the work to have survived the root change", works)
	}

	// Deleting the library leaves the work alone too.
	if err := repo.DeleteLibrary(library.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB().QueryRow("SELECT COUNT(*) FROM works").Scan(&works); err != nil {
		t.Fatal(err)
	}
	if works != 1 {
		t.Errorf("works = %d, want the work to have survived the library deletion", works)
	}
}

// TestMergeOutcomeReportsTheMergedCount keeps the port's count honest: it is the
// scan's own report, not a number the caller recomputes afterwards.
func TestMergeOutcomeReportsTheMergedCount(t *testing.T) {
	repo := newTestRepository(t)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"RJ00000001 album", "no number"} {
		stageDir(t, repo, "scan", "/music", name)
	}
	outcome, err := repo.MergeStaging("scan", library.ID, "/music")
	if err != nil {
		t.Fatal(err)
	}
	var want inventory.MergeOutcome
	_ = want
	if outcome.Merged != 2 {
		t.Errorf("merged = %d, want the two staged directories", outcome.Merged)
	}
}

// TestFindingAPlaceholderPromotesItWithoutLosingItsMetadata: a work a source
// declared is a placeholder until a scan finds it; promotion fills in when it
// was collected and leaves everything the import had already established.
func TestFindingAPlaceholderPromotesItWithoutLosingItsMetadata(t *testing.T) {
	repo := newTestRepository(t)
	coveringWork := seedWork(t, repo, "RJ00000001", true)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	const root = "/music"

	// The covering work declares a child the catalog has never seen on disk.
	if _, importErr := repo.ImportMetadata(&works.ValidatedImport{
		Target: coveringWork,
		Relations: &works.RelationsGroup{
			Source: works.SourceDLsite, FetchedAt: mustTime(t, testStamp),
			Role: works.RoleParent, ChildRJs: []works.WorkCode{code(t, "RJ00000002")},
		},
	}, mustTime(t, testStamp)); importErr != nil {
		t.Fatal(importErr)
	}
	placeholder, _, err := repo.GetWork(code(t, "RJ00000002"))
	if err != nil {
		t.Fatal(err)
	}
	if !placeholder.Placeholder {
		t.Fatal("the declared child was not created as a placeholder")
	}

	// A scan now finds a directory holding that work.
	stageDir(t, repo, "scan", root, "RJ00000002 the found copy")
	if _, mergeErr := repo.MergeStaging("scan", library.ID, root); mergeErr != nil {
		t.Fatal(mergeErr)
	}

	promoted, _, err := repo.GetWork(code(t, "RJ00000002"))
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Placeholder || promoted.CollectedAt == "" {
		t.Errorf(
			"the work was not promoted: placeholder=%v collected_at=%q",
			promoted.Placeholder,
			promoted.CollectedAt,
		)
	}
	// What the import had established survives the promotion.
	if promoted.Relation.ParentID == nil || *promoted.Relation.ParentID != coveringWork {
		t.Errorf("parent = %v, want the declared %s to have survived", promoted.Relation.ParentID, coveringWork)
	}
	// And it is now in the ordinary listing.
	page, err := repo.ListWorks(works.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Works) != 2 {
		t.Errorf(
			"the listing holds %d works, want both the covering work and the promoted placeholder",
			len(page.Works),
		)
	}
}

// A rescan repairs a binding to another work without replacing the copy or collection time.
func TestRescanRepairsWorkBindingWithoutReplacingCopy(t *testing.T) {
	repo := newTestRepository(t)
	library, err := repo.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	seedWork(t, repo, "RJ00000001", true)
	seedWork(t, repo, "RJ00000002", true)
	if _, err := repo.DB().Exec(`
		INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at)
		VALUES ('original-copy', 'RJ00000002', ?, 'RJ00000001 album', ?)
	`, library.ID, testStamp); err != nil {
		t.Fatal(err)
	}

	stageDir(t, repo, "rescan", "/music", "RJ00000001 album")
	if _, err := repo.MergeStaging("rescan", library.ID, "/music"); err != nil {
		t.Fatal(err)
	}
	var id, workID, created, collected string
	if err := repo.DB().QueryRow(`
		SELECT c.id, c.work_id, c.created_at, w.collected_at
		FROM work_copies c JOIN works w ON w.id = c.work_id
		WHERE c.library_id = ?
	`, library.ID).Scan(&id, &workID, &created, &collected); err != nil {
		t.Fatal(err)
	}
	if id != "original-copy" || workID != "RJ00000001" || created != testStamp || collected != testStamp {
		t.Fatalf("copy=%s work=%s created=%s collected=%s", id, workID, created, collected)
	}
}

func TestMergeBindsMoreThanSQLiteVariableLimit(t *testing.T) {
	repo := newTestRepository(t)
	library, createErr := repo.CreateLibrary("large", "/music")
	if createErr != nil {
		t.Fatal(createErr)
	}
	stageDir(t, repo, "before", "/music", "RJ00000001 album")
	stageDir(t, repo, "before", "/music", "RJ99999999 stale")
	if _, err := repo.MergeStaging("before", library.ID, "/music"); err != nil {
		t.Fatal(err)
	}
	var originalID string
	if err := repo.DB().
		QueryRow(`SELECT id FROM work_copies WHERE library_id = ? AND rel_path = 'RJ00000001 album'`, library.ID).
		Scan(&originalID); err != nil {
		t.Fatal(err)
	}

	const count = 32767
	store := NewScanStaging(repo)
	batch := make([]inventory.StagingEntry, 0, defaultBatchSize)
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("RJ%08d album", i)
		batch = append(batch, inventory.StagingEntry{
			SessionID: "large", RootPath: "/music", Path: "/music/" + name,
			ParentPath: "/music", Name: name, IsDir: true, IsWorkRoot: true,
		})
		if len(batch) == defaultBatchSize || i == count {
			if err := store.WriteStagingBatch("large", batch); err != nil {
				t.Fatal(err)
			}
			batch = batch[:0]
		}
	}
	outcome, err := repo.MergeStaging("large", library.ID, "/music")
	if err != nil {
		t.Fatal(err)
	}
	var copies, stale int
	var copyID string
	if err := repo.DB().
		QueryRow("SELECT COUNT(*) FROM work_copies WHERE library_id = ?", library.ID).
		Scan(&copies); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB().
		QueryRow("SELECT COUNT(*) FROM work_copies WHERE library_id = ? AND rel_path = 'RJ99999999 stale'", library.ID).
		Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB().
		QueryRow("SELECT id FROM work_copies WHERE library_id = ? AND rel_path = 'RJ00000001 album'", library.ID).
		Scan(&copyID); err != nil {
		t.Fatal(err)
	}
	if outcome.Merged != count || copies != count || stale != 0 || copyID != originalID {
		t.Fatalf(
			"merged=%d copies=%d stale=%d copy=%s, want %d and original copy %s",
			outcome.Merged,
			copies,
			stale,
			copyID,
			count,
			originalID,
		)
	}
}
