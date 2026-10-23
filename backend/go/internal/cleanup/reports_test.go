package cleanup_test

import (
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/cleanup"
)

func TestWorkerKeepsLargeReportGroupTogether(t *testing.T) {
	svc, store, _ := newCleanup(t, time.Millisecond, 0)
	for _, query := range []string{
		`WITH RECURSIVE ids(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM ids WHERE i<601) INSERT INTO relation_reports SELECT printf('RJ%08d',100000+i),'original',NULL,NULL,NULL,'dlsite','' FROM ids`,
		`WITH RECURSIVE ids(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM ids WHERE i<601) INSERT INTO relation_declared_children SELECT printf('RJ%08d',100000+i),printf('RJ%08d',100000+(i%601)+1) FROM ids`,
	} {
		if _, err := store.DB().Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Start(t.Context(), []cleanup.Category{cleanup.RelationReports}); err != nil {
		t.Fatal(err)
	}
	run := awaitTerminal(t, svc)
	svc.Shutdown()
	if run.Status != "succeeded" ||
		run.Counts[cleanup.RelationReports] != (cleanup.Count{Checked: 601, Deleted: 601}) ||
		store.batches != 1 {
		t.Fatalf("run=%+v batches=%d", run, store.batches)
	}
}

func TestWorkerDoesNotReclaimUnselectedReports(t *testing.T) {
	svc, store, _ := newCleanup(t, time.Millisecond, 0)
	if _, err := store.DB().
		Exec(`INSERT INTO relation_reports VALUES('RJ00000099','original',NULL,NULL,NULL,'dlsite','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions}); err != nil {
		t.Fatal(err)
	}
	run := awaitTerminal(t, svc)
	if run.Status != "succeeded" {
		t.Fatal(run)
	}
	ids, err := store.CleanupCandidates(t.Context(), cleanup.RelationReports, time.Now(), nil)
	if err != nil || len(ids) != 1 {
		t.Fatalf("unselected reports=%v err=%v", ids, err)
	}
}
