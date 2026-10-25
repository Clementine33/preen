package sqlite_test

import (
	"context"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/google/uuid"
	driverSQLite "modernc.org/sqlite"

	"github.com/Clementine33/preen/backend/internal/works"
)

func TestEditionTreeOmitsRejectedAndAmbiguousDeclarations(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	save := func(id works.WorkCode, report works.ImportRelations) {
		t.Helper()
		report.Source, report.FetchedAt = "dlsite", deletionStamp
		if _, err := catalog.ImportRelationReport(id, report); err != nil {
			t.Fatal(err)
		}
	}
	save(
		deletionWork,
		works.ImportRelations{
			Role:     "parent",
			Language: new("CHI_HANS"),
			ChildRJs: []string{"RJ00000002", "RJ00000004", "RJ00000005"},
		},
	)
	save("RJ00000002", works.ImportRelations{Role: "child", ParentRJ: new("RJ00000003")})
	save("RJ00000003", works.ImportRelations{Role: "parent", ChildRJs: []string{"RJ00000005"}})
	detail, found, err := repo.GetWork(deletionWork)
	if err != nil || !found {
		t.Fatalf("detail: %v %v", found, err)
	}
	if len(detail.EditionTree) != 1 {
		t.Fatalf("tree: %+v", detail.EditionTree)
	}
	kids := detail.EditionTree[0].Translations
	if len(detail.RelationWarnings) != 2 {
		t.Fatalf("warnings = %+v", detail.RelationWarnings)
	}
	if len(kids) != 1 || kids[0].RJ != "RJ00000004" {
		t.Fatalf("definite children = %+v, want only external RJ00000004", kids)
	}
}

func TestDetailKeepsExternalBridgeAndKnownFacts(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	page, original := works.WorkCode("RJ01289610"), works.WorkCode("RJ01287230")
	for id, report := range map[works.WorkCode]works.ImportRelations{
		page:         {Role: "parent", Language: new("CHI_HANS"), OriginalRJ: new(string(original)), ParentRJ: new(string(original))},
		deletionWork: {Role: "child", ParentRJ: new(string(page))},
	} {
		report.Source, report.FetchedAt = "dlsite", deletionStamp
		if _, err := catalog.ImportRelationReport(id, report); err != nil {
			t.Fatal(err)
		}
	}
	detail, _, err := repo.GetWork(deletionWork)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.EditionTree) != 2 {
		t.Fatalf("bridge tree: %+v", detail.EditionTree)
	}
	external := detail.EditionTree[1].Root
	if external.RJ != page || external.IsWork || external.Held || external.Language == nil ||
		*external.Language != "CHI_HANS" ||
		external.Relation.ParentID == nil ||
		*external.Relation.ParentID != original {
		t.Fatalf("external bridge: %+v", external)
	}
	if external.SourceURL == nil ||
		*external.SourceURL != "https://www.dlsite.com/maniax/work/=/product_id/RJ01289610.html" {
		t.Fatalf("source URL: %v", external.SourceURL)
	}
	unknown := detail.EditionTree[0].Root
	if unknown.Language != nil || !unknown.Relation.IsEmpty() {
		t.Fatalf("unknown original metadata was invented: %+v", unknown)
	}
}

// A SQL view pauses the first SELECT after SQLite has established its snapshot.
// The writer must commit while paused, so write-lock serialization cannot pass.
func TestDetailUsesOneDeferredReadSnapshot(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	function := "detail_barrier_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := driverSQLite.RegisterScalarFunction(
		function,
		1,
		func(_ *driverSQLite.FunctionContext, args []driver.Value) (driver.Value, error) {
			once.Do(func() { close(entered); <-release })
			return args[0], nil
		},
	); err != nil {
		t.Fatal(err)
	}
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	for id, report := range map[works.WorkCode]works.ImportRelations{
		deletionWork: {Role: "child", ParentRJ: new("RJ00000002"), OriginalRJ: new("RJ00000003"), Language: new("CHI_HANS")},
		"RJ00000002": {Role: "parent", OriginalRJ: new("RJ00000003"), Language: new("CHI_HANS")},
	} {
		report.Source, report.FetchedAt = "dlsite", deletionStamp
		if _, err := catalog.ImportRelationReport(id, report); err != nil {
			t.Fatal(err)
		}
	}
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000003',?,?)`,
		deletionStamp,
		deletionStamp,
	)
	deletionSQL(t, repo, `INSERT INTO libraries(id,root_path) VALUES('snapshot-library','/snapshot')`)
	deletionSQL(
		t,
		repo,
		`INSERT INTO work_copies VALUES('snapshot-old-copy','RJ00000001','snapshot-library','old','now')`,
	)
	inventoryfixture.SeedInventory(t, repo,
		inventoryfixture.Row{Path: "/snapshot/old/cover.jpg", Size: 10, Mtime: 20},
		inventoryfixture.Row{Path: "/snapshot-new/work/cover.png", Size: 30, Mtime: 40, ContentRev: 2},
	)
	deletionSQL(t, repo, `INSERT INTO circles(id,source,name) VALUES('snapshot-circle','dlsite','circle')`)
	deletionSQL(t, repo, `UPDATE works SET title='old',circle_id='snapshot-circle' WHERE id=?`, string(deletionWork))
	deletionSQL(
		t,
		repo,
		`INSERT INTO work_language_editions(work_id,version_root_rj,language,display_order) VALUES(?,'RJ00000002','CHI_HANS',1)`,
		string(deletionWork),
	)
	deletionSQL(t, repo, `ALTER TABLE circles RENAME TO snapshot_circles`)
	deletionSQL(
		t,
		repo,
		`CREATE VIEW circles AS SELECT id,source,source_circle_id,`+function+`(name) AS name,work_id FROM snapshot_circles`,
	)
	read := make(chan detailReadResult, 1)
	go func() { d, _, err := repo.GetWork(deletionWork); read <- detailReadResult{d, err} }()
	defer close(release)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("read did not reach barrier")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := repo.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("writer blocked by read: %v", err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		`UPDATE works SET title='new' WHERE id='RJ00000001'`,
		`UPDATE relation_reports SET parent_rj='RJ00000004' WHERE subject_rj='RJ00000001'`,
		`UPDATE work_translations SET parent_work_id='RJ00000004' WHERE work_id='RJ00000001'`,
		`UPDATE work_language_editions SET version_root_rj='RJ00000004' WHERE work_id='RJ00000001'`,
		`DELETE FROM works WHERE id='RJ00000003'`,
		`DELETE FROM work_copies WHERE work_id='RJ00000001'`,
		`UPDATE libraries SET root_path='/snapshot-new' WHERE id='snapshot-library'`,
		`INSERT INTO work_copies VALUES('snapshot-copy','RJ00000001','snapshot-library','work','now')`,
	} {
		if _, execErr := tx.ExecContext(ctx, query); execErr != nil {
			t.Fatal(execErr)
		}
	}
	if commitErr := tx.Commit(); commitErr != nil {
		t.Fatalf("writer did not commit during read: %v", commitErr)
	}
	release <- struct{}{}
	got := <-read
	if got.err != nil {
		t.Fatal(got.err)
	}
	assertOldDetailSnapshot(t, got.detail)
	latest, _, err := repo.GetWork(deletionWork)
	if err != nil {
		t.Fatal(err)
	}
	assertNewDetailSnapshot(t, latest)
}

func assertOldDetailSnapshot(t *testing.T, d *works.WorkDetail) {
	t.Helper()
	if d.Title == nil || *d.Title != "old" || d.CopyCount != 1 || len(d.Copies) != 1 ||
		d.Copies[0].ID != "snapshot-old-copy" ||
		d.Copies[0].LibraryRoot != "/snapshot" ||
		d.LocalCover == nil ||
		d.LocalCover.CopyID != "snapshot-old-copy" ||
		d.LocalCover.RelPath != "cover.jpg" ||
		d.LocalCover.Version != "1-20-10" ||
		*d.Relation.ParentID != "RJ00000002" ||
		d.Editions[0].VersionRootRJ != "RJ00000002" ||
		!d.EditionTree[0].Root.IsWork ||
		!d.EditionTree[1].Translations[0].Held {
		t.Fatalf("mixed snapshot: %+v, tree=%+v", d, d.EditionTree)
	}
}

func assertNewDetailSnapshot(t *testing.T, latest *works.WorkDetail) {
	t.Helper()
	if *latest.Title != "new" || latest.CopyCount != 1 || len(latest.Copies) != 1 ||
		latest.Copies[0].ID != "snapshot-copy" ||
		latest.Copies[0].LibraryRoot != "/snapshot-new" ||
		latest.LocalCover == nil ||
		latest.LocalCover.CopyID != "snapshot-copy" ||
		latest.LocalCover.RelPath != "cover.png" ||
		latest.LocalCover.Version != "2-40-30" ||
		*latest.Relation.ParentID != "RJ00000004" ||
		latest.Editions[0].VersionRootRJ != "RJ00000004" ||
		latest.EditionTree[0].Root.IsWork ||
		!latest.EditionTree[1].Translations[0].Held {
		t.Fatalf("new snapshot: %+v", latest)
	}
}

func TestDetailPreservesLanguageFreeExternalParentChain(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	for id, report := range map[works.WorkCode]works.ImportRelations{
		deletionWork: {Role: "child", ParentRJ: new("RJ00000002")},
		"RJ00000002": {Role: "parent", ParentRJ: new("RJ00000003"), OriginalRJ: new("RJ00000004")},
		"RJ00000003": {Role: "parent", ParentRJ: new("RJ00000004"), OriginalRJ: new("RJ00000004")},
	} {
		report.Source, report.FetchedAt = "dlsite", deletionStamp
		if _, err := catalog.ImportRelationReport(id, report); err != nil {
			t.Fatal(err)
		}
	}
	detail, _, err := repo.GetWork(deletionWork)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.RelationNodes) != 4 || len(detail.EditionTree) != 1 {
		t.Fatalf("language-free chain: %+v", detail)
	}
	for i, want := range []works.WorkCode{deletionWork, "RJ00000002", "RJ00000003", "RJ00000004"} {
		node := detail.RelationNodes[i]
		if node.RJ != want || node.Language != nil || node.IsWork != (i == 0) {
			t.Fatalf("node %d: %+v", i, node)
		}
	}
	if *detail.RelationNodes[1].Relation.ParentID != "RJ00000003" ||
		*detail.RelationNodes[2].Relation.ParentID != "RJ00000004" {
		t.Fatalf("skipped external ancestor: %+v", detail.RelationNodes)
	}
}

type detailReadResult struct {
	detail *works.WorkDetail
	err    error
}
