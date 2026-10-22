package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

func TestRelationReportDoesNotRequireCatalogWork(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	external := works.WorkCode("RJ00000002")
	notified := 0
	repo.OnCatalogChanged = func() { notified++ }
	outcome, err := catalog.ImportRelationReport(external, works.ImportRelations{
		Source: "dlsite", FetchedAt: deletionStamp, Role: "parent", ChildRJs: []string{string(deletionWork)},
	})
	if err != nil || len(outcome.Applied) != 1 {
		t.Fatalf("report: %+v, %v", outcome, err)
	}
	if _, found, err := repo.GetWork(external); err != nil || found {
		t.Fatalf("external became work: %v, %v", found, err)
	}
	detail, found, lookupErr := repo.GetWork(deletionWork)
	if lookupErr != nil || !found || detail.Relation.ParentID == nil || *detail.Relation.ParentID != external {
		t.Fatalf("local relation: %+v, %v", detail, lookupErr)
	}
	states, stateErr := repo.MetadataStates(context.Background(), []works.WorkCode{external})
	if stateErr != nil || states[external][works.GroupRelations].Source != works.SourceDLsite {
		t.Fatalf("external report state: %+v, %v", states, stateErr)
	}
	if notified != 1 {
		t.Fatalf("notifications = %d", notified)
	}
	_, importErr := catalog.Import(external, works.ImportRequest{Relations: &works.ImportRelations{
		Source: "dlsite", FetchedAt: deletionStamp, Role: "original",
	}})
	if importErr == nil || importErr.Code != works.CodeWorkNotFound {
		t.Fatalf("full import: %v", importErr)
	}
}

func TestRelationReportsSurviveWorkDeletionAndKeepFreshness(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	original := works.WorkCode("RJ00000002")
	page := works.WorkCode("RJ00000003")
	replacement := works.WorkCode("RJ00000004")
	save := func(id works.WorkCode, report works.ImportRelations) works.ImportOutcome {
		t.Helper()
		out, err := catalog.ImportRelationReport(id, report)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	pageReport := works.ImportRelations{
		Source:     "dlsite",
		FetchedAt:  deletionStamp,
		Role:       "parent",
		OriginalRJ: new(string(original)),
		Language:   new("CHI_HANS"),
	}
	save(page, pageReport)
	localReport := works.ImportRelations{
		Source:    "dlsite",
		FetchedAt: deletionStamp,
		Role:      "child",
		ParentRJ:  new(string(page)),
	}
	save(deletionWork, localReport)
	assertLocal := func(want works.WorkCode) {
		t.Helper()
		detail, found, err := repo.GetWork(deletionWork)
		if err != nil || !found {
			t.Fatalf("detail: %v %v", found, err)
		}
		relation := detail.Relation
		if relation.ParentID == nil || *relation.ParentID != page || relation.OriginalID == nil ||
			*relation.OriginalID != want ||
			relation.Language == nil ||
			*relation.Language != "CHI_HANS" {
			t.Fatalf("relation = %+v, want parent %s and original %s", relation, page, want)
		}
	}
	assertLocal(original)
	pageReport.FetchedAt = "2026-09-28T10:00:00.000000000Z"
	pageReport.OriginalRJ = new(string(replacement))
	save(page, pageReport)
	assertLocal(replacement)
	// A target may enter and leave the catalog without changing the report.
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,created_at,updated_at) VALUES(?,?,?,?)`,
		string(replacement),
		deletionStamp,
		deletionStamp,
		deletionStamp,
	)
	if refusal := catalog.Delete(replacement); refusal != nil {
		t.Fatal(refusal)
	}
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,created_at,updated_at) VALUES(?,?,?,?)`,
		string(page),
		deletionStamp,
		deletionStamp,
		deletionStamp,
	)
	if refusal := catalog.Delete(page); refusal != nil {
		t.Fatal(refusal)
	}
	assertLocal(replacement)
	if refusal := catalog.Delete(deletionWork); refusal != nil {
		t.Fatal(refusal)
	}
	states, err := repo.MetadataStates(context.Background(), []works.WorkCode{deletionWork, page})
	if err != nil || states[deletionWork][works.GroupRelations].Source != works.SourceDLsite ||
		states[page][works.GroupRelations].Source != works.SourceDLsite {
		t.Fatalf("states after deletion: %+v, %v", states, err)
	}
	stale := save(deletionWork, localReport)
	if len(stale.Ignored) != 1 {
		t.Fatalf("deleted subject lost freshness: %+v", stale)
	}
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,created_at,updated_at) VALUES(?,?,?,?)`,
		string(deletionWork),
		deletionStamp,
		deletionStamp,
		deletionStamp,
	)
	// A later accepted ancestor report rebuilds only extant owners.
	pageReport.FetchedAt = "2026-09-29T10:00:00.000000000Z"
	save(page, pageReport)
	assertLocal(replacement)
}

func TestRelationReportReplacementUsesItsOwnSourceState(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	external := works.WorkCode("RJ00000002")
	report := works.ImportRelations{
		Source:    "asmr_one",
		FetchedAt: "2026-09-29T10:00:00.000000000Z",
		Role:      "parent",
		ChildRJs:  []string{string(deletionWork)},
	}
	if _, err := catalog.ImportRelationReport(external, report); err != nil {
		t.Fatal(err)
	}
	// Higher source priority wins even when its fetch is older, and a complete
	// empty child snapshot removes the declaration without removing the report.
	report.Source, report.FetchedAt, report.ChildRJs = "dlsite", deletionStamp, nil
	out, err := catalog.ImportRelationReport(external, report)
	if err != nil || len(out.Applied) != 1 {
		t.Fatalf("priority replacement: %+v, %v", out, err)
	}
	detail, _, lookupErr := repo.GetWork(deletionWork)
	if lookupErr != nil || !detail.Relation.IsEmpty() {
		t.Fatalf("child snapshot survived: %+v, %v", detail, lookupErr)
	}
	for _, tc := range []struct{ source, stamp, reason string }{
		{"dlsite", deletionStamp, works.ReasonStaleFetchedAt},
		{"dlsite", "2026-09-26T10:00:00.000000000Z", works.ReasonStaleFetchedAt},
		{"asmr_one", "2026-09-30T10:00:00.000000000Z", works.ReasonLowerPrioritySource},
	} {
		report.Source, report.FetchedAt = tc.source, tc.stamp
		report.ChildRJs = []string{string(deletionWork)}
		out, err = catalog.ImportRelationReport(external, report)
		if err != nil || len(out.Ignored) != 1 || out.Ignored[0].Reason != tc.reason {
			t.Fatalf("ignored: %+v, %v", out, err)
		}
	}
	states, stateErr := repo.MetadataStates(context.Background(), []works.WorkCode{external})
	if stateErr != nil || states[external][works.GroupRelations].Source != works.SourceDLsite {
		t.Fatalf("empty report state: %+v, %v", states, stateErr)
	}
	// The ordinary import consults the same report authority after membership changes.
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,created_at,updated_at) VALUES(?,?,?,?)`,
		string(external),
		deletionStamp,
		deletionStamp,
		deletionStamp,
	)
	report.Source, report.FetchedAt = "dlsite", deletionStamp
	out, err = catalog.Import(external, works.ImportRequest{Relations: &report})
	if err != nil || len(out.Ignored) != 1 || out.Ignored[0].Reason != works.ReasonStaleFetchedAt {
		t.Fatalf("ordinary import freshness: %+v, %v", out, err)
	}
	detail, _, lookupErr = repo.GetWork(deletionWork)
	if lookupErr != nil || !detail.Relation.IsEmpty() {
		t.Fatalf("stale declaration was restored: %+v, %v", detail, lookupErr)
	}
}

func TestRelationReportFailureRollsBackEvidenceStateAndDerivedValues(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	external := works.WorkCode("RJ00000002")
	original := "RJ00000003"
	other := works.WorkCode("RJ00000004")
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,created_at,updated_at) VALUES(?,?,?,?)`,
		string(other),
		deletionStamp,
		deletionStamp,
		deletionStamp,
	)
	report := works.ImportRelations{
		Source:     "dlsite",
		FetchedAt:  deletionStamp,
		Role:       "parent",
		OriginalRJ: &original,
		ChildRJs:   []string{string(deletionWork)},
	}
	if _, err := catalog.ImportRelationReport(external, report); err != nil {
		t.Fatal(err)
	}
	deletionSQL(
		t,
		repo,
		`CREATE TRIGGER refuse_relation_update BEFORE UPDATE ON work_translations BEGIN SELECT RAISE(ABORT,'injected failure'); END`,
	)
	notified := 0
	repo.OnCatalogChanged = func() { notified++ }
	report.FetchedAt = "2026-09-28T10:00:00.000000000Z"
	report.OriginalRJ = new("RJ00000005")
	report.ChildRJs = append(report.ChildRJs, string(other))
	if _, err := catalog.ImportRelationReport(external, report); err == nil || err.Kind != works.KindInternal {
		t.Fatalf("injected write failure: %v", err)
	}
	if notified != 0 {
		t.Fatal("failed report notified")
	}
	detail, _, err := repo.GetWork(deletionWork)
	if err != nil || detail.Relation.OriginalID == nil || string(*detail.Relation.OriginalID) != original {
		t.Fatalf("derived rollback: %+v, %v", detail, err)
	}
	second, _, err := repo.GetWork(other)
	if err != nil || !second.Relation.IsEmpty() {
		t.Fatalf("new child survived rollback: %+v, %v", second, err)
	}
	states, stateErr := repo.MetadataStates(context.Background(), []works.WorkCode{external})
	before, _ := time.Parse(works.TimestampFormat, deletionStamp)
	if stateErr != nil || states[external][works.GroupRelations].FetchedAt != before.UnixNano() {
		t.Fatalf("freshness rollback: %+v, %v", states, stateErr)
	}
	deletionSQL(t, repo, `DROP TRIGGER refuse_relation_update`)
	out, refusal := catalog.ImportRelationReport(external, report)
	if refusal != nil || len(out.Applied) != 1 {
		t.Fatalf("retry after rollback: %+v, %v", out, refusal)
	}
	second, _, err = repo.GetWork(other)
	if err != nil || second.Relation.ParentID == nil || *second.Relation.ParentID != external {
		t.Fatalf("retried children: %+v, %v", second, err)
	}
}

func TestRelationReportRejectsInvalidStructureWithoutCommitting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    works.ImportRelations
		incoming works.ImportRelations
		code     string
	}{
		{"self parent", works.ImportRelations{Role: "original"}, works.ImportRelations{Role: "child", ParentRJ: new("RJ00000003")}, works.CodeRelationSelfReference},
		{"self original", works.ImportRelations{Role: "original"}, works.ImportRelations{Role: "child", OriginalRJ: new("RJ00000003")}, works.CodeRelationSelfReference},
		{"original chain", works.ImportRelations{Role: "child", OriginalRJ: new("RJ00000004")}, works.ImportRelations{Role: "child", OriginalRJ: new("RJ00000002")}, works.CodeRelationOriginalChain},
		{"effective parent cycle", works.ImportRelations{Role: "parent", ParentRJ: new("RJ00000003")}, works.ImportRelations{Role: "parent", ParentRJ: new("RJ00000002")}, works.CodeRelationCycle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := deletionRepository(t)
			catalog := works.NewService(repo, nil, nil, nil, "")
			parent, subject := works.WorkCode("RJ00000002"), works.WorkCode("RJ00000003")
			tc.setup.Source, tc.setup.FetchedAt = "dlsite", deletionStamp
			if _, err := catalog.ImportRelationReport(parent, tc.setup); err != nil {
				t.Fatal(err)
			}
			notified := 0
			repo.OnCatalogChanged = func() { notified++ }
			tc.incoming.Source, tc.incoming.FetchedAt = "dlsite", deletionStamp
			if _, err := catalog.ImportRelationReport(subject, tc.incoming); err == nil || err.Code != tc.code {
				t.Fatalf("refusal = %v, want %s", err, tc.code)
			}
			states, err := repo.MetadataStates(context.Background(), []works.WorkCode{subject})
			if err != nil || len(states[subject]) != 0 || notified != 0 {
				t.Fatalf("refused report survived: %+v, notifications=%d, %v", states, notified, err)
			}
		})
	}
}

func TestRelationReportsKeepAmbiguousClaimsAndPreferSelfReport(t *testing.T) {
	repo := deletionRepository(t)
	catalog := works.NewService(repo, nil, nil, nil, "")
	for _, id := range []works.WorkCode{"RJ00000002", "RJ00000003"} {
		out, err := catalog.ImportRelationReport(
			id,
			works.ImportRelations{
				Source:    "dlsite",
				FetchedAt: deletionStamp,
				Role:      "parent",
				ChildRJs:  []string{string(deletionWork)},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if id == "RJ00000003" && (len(out.Warnings) != 1 || out.Warnings[0].Code != works.CodeRelationAmbiguous) {
			t.Fatalf("ambiguity warnings: %+v", out.Warnings)
		}
	}
	detail, _, err := repo.GetWork(deletionWork)
	if err != nil || detail.Relation.ParentID != nil {
		t.Fatalf("ambiguous parent: %+v, %v", detail, err)
	}
	parent := "RJ00000002"
	out, refusal := catalog.ImportRelationReport(
		deletionWork,
		works.ImportRelations{Source: "dlsite", FetchedAt: deletionStamp, Role: "child", ParentRJ: &parent},
	)
	if refusal != nil || len(out.Warnings) != 1 || out.Warnings[0].Code != works.CodeRelationConflict {
		t.Fatalf("self report warnings: %+v, %v", out, refusal)
	}
	detail, _, err = repo.GetWork(deletionWork)
	if err != nil || detail.Relation.ParentID == nil || string(*detail.Relation.ParentID) != parent {
		t.Fatalf("self parent: %+v, %v", detail, err)
	}
}
