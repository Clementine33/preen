package sqlite_test

import (
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/cleanup"
)

func startCatalogRun(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	if err := repo.CreateCleanupRun(
		t.Context(),
		cleanup.Run{ID: "catalog", Status: "running", Counts: map[cleanup.Category]cleanup.Count{}},
	); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupWorksRechecksEveryProtectionAfterPreview(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"rating", `INSERT INTO work_user_data(work_id,rating) VALUES('RJ00000001',1)`},
		{"note", `INSERT INTO work_user_data(work_id,note) VALUES('RJ00000001','remember')`},
		{"whitespace note", `INSERT INTO work_user_data(work_id,note) VALUES('RJ00000001',' ')`},
		{"user tag", `INSERT INTO user_tags VALUES('tag','keep'); INSERT INTO work_user_tags VALUES('RJ00000001','tag')`},
		{"favorites", `INSERT INTO collection_works VALUES('favorites','RJ00000001',0)`},
		{"collection", `INSERT INTO collections VALUES('custom','keep','',''); INSERT INTO collection_works VALUES('custom','RJ00000001',0)`},
		{"offline copy", `INSERT INTO libraries(id,name,root_path,created_at,updated_at) VALUES('offline','Offline','/nonexistent-preen-library','',''); INSERT INTO work_copies VALUES('copy','RJ00000001','offline','work','')`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := deletionRepository(t)
			svc := catalogCleanup(t, repo)
			preview, err := svc.Preview(t.Context(), []cleanup.Category{cleanup.Works})
			if err != nil || preview.Candidates[cleanup.Works] != 1 {
				t.Fatalf("before: %+v %v", preview, err)
			}
			ids, err := repo.CleanupCandidates(t.Context(), cleanup.Works, time.Now(), nil)
			if err != nil {
				t.Fatal(err)
			}
			deletionSQL(t, repo, tc.sql)
			preview, err = svc.Preview(t.Context(), []cleanup.Category{cleanup.Works})
			if err != nil || preview.Candidates[cleanup.Works] != 0 {
				t.Fatalf("protected: %+v %v", preview, err)
			}
			notifications := 0
			repo.OnCatalogChanged = func() { notifications++ }
			startCatalogRun(t, repo)
			if batchErr := repo.CleanupBatch(t.Context(), "catalog", cleanup.Works, time.Now(), ids); batchErr != nil {
				t.Fatal(batchErr)
			}
			saved, err := repo.ReadCleanupRun(t.Context())
			if err != nil || saved.Counts[cleanup.Works] != (cleanup.Count{Checked: 1, Skipped: 1}) {
				t.Fatalf("counts: %+v %v", saved, err)
			}
			if _, found, err := repo.GetWork(deletionWork); err != nil || !found || notifications != 0 {
				t.Fatalf("protected work: %v %v notifications=%d", found, err, notifications)
			}
		})
	}
}

func TestCleanupWorksRacesWithPersonalEdits(t *testing.T) {
	for _, kind := range []string{"rating", "note", "favorite", "tags", "collection"} {
		t.Run(kind, func(t *testing.T) {
			repo := deletionRepository(t)
			tag, _, err := repo.CreateUserTag("keep")
			if err != nil {
				t.Fatal(err)
			}
			startCatalogRun(t, repo)
			start := make(chan struct{})
			edited, deleted := make(chan error, 1), make(chan error, 1)
			go func() { <-start; edited <- editDeletionWork(repo, "service", kind, tag.ID) }()
			go func() {
				<-start
				deleted <- repo.CleanupBatch(t.Context(), "catalog", cleanup.Works, time.Now(), []string{string(deletionWork)})
			}()
			close(start)
			editErr, deleteErr := <-edited, <-deleted
			if deleteErr != nil {
				t.Fatal(deleteErr)
			}
			saved, err := repo.ReadCleanupRun(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if editErr == nil {
				if saved.Counts[cleanup.Works] != (cleanup.Count{Checked: 1, Skipped: 1}) {
					t.Fatalf("saved edit but deleted: %+v", saved)
				}
			} else {
				assertMissingWork(t, editErr)
				if saved.Counts[cleanup.Works] != (cleanup.Count{Checked: 1, Deleted: 1}) {
					t.Fatalf("deleted: %+v", saved)
				}
			}
		})
	}
}

func TestCleanupWorksKeepsFiniteCandidatesAndRollsBackOnlyFailingBatch(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(t, repo, `INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000002','','')`)
	ids, err := repo.CleanupCandidates(t.Context(), cleanup.Works, time.Now(), nil)
	if err != nil || len(ids) != 2 {
		t.Fatalf("candidates: %v %v", ids, err)
	}
	deletionSQL(t, repo, `INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000003','','')`)
	startCatalogRun(t, repo)
	notifications := 0
	repo.OnCatalogChanged = func() { notifications++ }
	if batchErr := repo.CleanupBatch(t.Context(), "catalog", cleanup.Works, time.Now(), ids[:1]); batchErr != nil {
		t.Fatal(batchErr)
	}
	deletionSQL(
		t,
		repo,
		`CREATE TRIGGER reject_cleanup_progress BEFORE UPDATE ON cleanup_run BEGIN SELECT RAISE(ABORT,'injected'); END`,
	)
	if batchErr := repo.CleanupBatch(t.Context(), "catalog", cleanup.Works, time.Now(), ids[1:]); batchErr == nil {
		t.Fatal("expected failed batch")
	}
	saved, err := repo.ReadCleanupRun(t.Context())
	if err != nil || saved.Counts[cleanup.Works] != (cleanup.Count{Checked: 1, Deleted: 1}) {
		t.Fatalf("committed counts: %+v %v", saved, err)
	}
	for _, id := range []string{"RJ00000002", "RJ00000003"} {
		var count int
		if err := repo.DB().
			QueryRow("SELECT COUNT(*) FROM works WHERE id=?", id).
			Scan(&count); err != nil ||
			count != 1 {
			t.Fatalf("remaining work %s: %d %v", id, count, err)
		}
	}
	if notifications != 1 {
		t.Fatalf("notifications=%d", notifications)
	}
}
