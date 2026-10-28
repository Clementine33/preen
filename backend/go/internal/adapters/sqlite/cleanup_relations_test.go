package sqlite_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/works"
)

func TestCleanupRelationPreviewProjectsWorkSelection(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(
		t,
		repo,
		`INSERT INTO relation_reports VALUES('RJ00000001','child',NULL,'RJ00000002',NULL,'dlsite',''),('RJ00000002','original',NULL,NULL,NULL,'dlsite','')`,
	)
	svc := catalogCleanup(t, repo)
	preview, err := svc.Preview(t.Context(), []cleanup.Category{cleanup.RelationReports})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Candidates[cleanup.RelationReports] != 1 {
		t.Fatalf("work root: %+v", preview)
	}
	preview, err = svc.Preview(t.Context(), []cleanup.Category{cleanup.RelationReports, cleanup.Works})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Candidates[cleanup.RelationReports] != 2 {
		t.Fatalf("projected root: %+v", preview)
	}
	ids, err := repo.CleanupCandidates(t.Context(), cleanup.RelationReports, time.Now(), nil)
	if err != nil || !reflect.DeepEqual(ids, []string{"RJ00000002"}) {
		t.Fatalf("ids=%v %v", ids, err)
	}
}

func saveCleanupReport(t *testing.T, repo *sqlite.Repository, id string, report works.ImportRelations) {
	t.Helper()
	report.Source, report.FetchedAt = "dlsite", deletionStamp
	if _, err := works.NewService(repo, nil, nil, nil, "").
		ImportRelationReport(works.WorkCode(id), report); err != nil {
		t.Fatal(err)
	}
}

func cleanupEvidence(t *testing.T, repo *sqlite.Repository) works.Evidence {
	t.Helper()
	ev := works.Evidence{
		Self:             map[works.WorkCode]works.RelationSelf{},
		DeclaredChildren: map[works.WorkCode][]works.WorkCode{},
	}
	rows, err := repo.DB().Query("SELECT subject_rj,role,original_rj,parent_rj FROM relation_reports")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id works.WorkCode
		var role works.RelationRole
		var original, parent *works.WorkCode
		if scanErr := rows.Scan(&id, &role, &original, &parent); scanErr != nil {
			t.Fatal(scanErr)
		}
		ev.Self[id] = works.RelationSelf{Role: role, OriginalRJ: original, ParentRJ: parent}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		t.Fatal(rowsErr)
	}
	if closeErr := rows.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	rows, err = repo.DB().Query("SELECT declaring_rj,child_rj FROM relation_declared_children")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, child works.WorkCode
		if err := rows.Scan(&id, &child); err != nil {
			t.Fatal(err)
		}
		ev.DeclaredChildren[id] = append(ev.DeclaredChildren[id], child)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		t.Fatal(rowsErr)
	}
	return ev
}

func TestCleanupRelationsFixedPointConflictsAndEveryCommit(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000007','',''),('RJ00000008','','')`,
	)
	for _, id := range []string{"RJ00000001", "RJ00000003", "RJ00000006", "RJ00000011", "RJ00000012"} {
		saveCleanupReport(t, repo, id, works.ImportRelations{Role: "original"})
	}
	saveCleanupReport(
		t,
		repo,
		"RJ00000002",
		works.ImportRelations{
			Role:     "child",
			ParentRJ: new("RJ00000003"),
			ChildRJs: []string{"RJ00000001", "RJ00000003", "RJ00000011"},
		},
	)
	saveCleanupReport(
		t,
		repo,
		"RJ00000004",
		works.ImportRelations{Role: "original", ChildRJs: []string{"RJ00000005"}, FetchedAt: deletionStamp},
	)
	saveCleanupReport(t, repo, "RJ00000005", works.ImportRelations{Role: "original", ChildRJs: []string{"RJ00000004"}})
	saveCleanupReport(t, repo, "RJ00000007", works.ImportRelations{Role: "child", OriginalRJ: new("RJ00000006")})
	saveCleanupReport(t, repo, "RJ00000009", works.ImportRelations{Role: "parent", ChildRJs: []string{"RJ00000008"}})
	saveCleanupReport(t, repo, "RJ00000010", works.ImportRelations{Role: "parent", ChildRJs: []string{"RJ00000008"}})
	before := works.ResolveRoots(cleanupEvidence(t, repo), []works.WorkCode{deletionWork, "RJ00000007", "RJ00000008"})
	if len(before.Warnings) != 2 {
		t.Fatalf("warnings=%v", before.Warnings)
	}
	ids, err := repo.CleanupCandidates(t.Context(), cleanup.RelationReports, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	if !reflect.DeepEqual(sorted, []string{"RJ00000004", "RJ00000005", "RJ00000006", "RJ00000011", "RJ00000012"}) {
		t.Fatalf("candidates=%v", ids)
	}
	groups, err := repo.CleanupGroups(t.Context(), cleanup.RelationReports, ids)
	if err != nil {
		t.Fatal(err)
	}
	if createErr := repo.CreateCleanupRun(
		t.Context(),
		cleanup.Run{ID: "reports", Status: "running", Counts: make(map[cleanup.Category]cleanup.Count)},
	); createErr != nil {
		t.Fatal(createErr)
	}
	notifications := 0
	repo.OnCatalogChanged = func() { notifications++ }
	for _, group := range groups {
		if slices.Contains(group, "RJ00000004") && len(group) != 2 {
			t.Fatalf("mutual group split: %v", groups)
		}
		if batchErr := repo.CleanupBatch(
			t.Context(),
			"reports",
			cleanup.RelationReports,
			time.Now(),
			group,
		); batchErr != nil {
			t.Fatal(batchErr)
		}
		ev := cleanupEvidence(t, repo)
		if violation := works.ValidateRelations(ev); violation != nil {
			t.Fatal(violation)
		}
		after := works.ResolveRoots(ev, []works.WorkCode{deletionWork, "RJ00000007", "RJ00000008"})
		if !reflect.DeepEqual(before.Relations, after.Relations) ||
			!reflect.DeepEqual(before.Warnings, after.Warnings) {
			t.Fatalf("changed root results: before=%+v after=%+v", before, after)
		}
	}
	run, err := repo.ReadCleanupRun(t.Context())
	if err != nil || run.Counts[cleanup.RelationReports] != (cleanup.Count{Checked: 5, Deleted: 5}) ||
		notifications != len(groups) {
		t.Fatalf("run=%+v notifications=%d err=%v", run, notifications, err)
	}
}
