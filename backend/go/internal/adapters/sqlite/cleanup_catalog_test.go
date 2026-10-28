package sqlite_test

import (
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
)

func catalogCleanup(t *testing.T, repo *sqlite.Repository) *cleanup.Service {
	t.Helper()
	svc := cleanup.NewService(t.Context(), repo, admission.NewGate(nil), cleanup.Options{})
	t.Cleanup(svc.Shutdown)
	return svc
}

func TestCleanupPreviewProjectsOnlySelectedWorksAndExcludesOwnedSources(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(t, repo, `INSERT INTO works(id,collected_at,updated_at) VALUES('RJ00000002','','')`)
	for _, table := range []string{"tags", "voice_actors", "circles"} {
		sourceColumn := map[string]string{"tags": "source_tag_id", "voice_actors": "source_voice_actor_id", "circles": "source_circle_id"}[table]
		deletionSQL(
			t,
			repo,
			"INSERT INTO "+table+"(id,source,"+sourceColumn+",name,work_id) VALUES(?, 'dlsite', 'shared', 'shared', NULL), (?, 'dlsite', 'kept', 'kept', NULL), (?, 'dlsite', NULL, 'owned', 'RJ00000001')",
			table+"-shared",
			table+"-kept",
			table+"-owned",
		)
	}
	deletionSQL(
		t,
		repo,
		`UPDATE works SET circle_id='circles-shared' WHERE id='RJ00000001'; UPDATE works SET circle_id='circles-kept' WHERE id='RJ00000002';
 INSERT INTO work_tags VALUES('RJ00000001','tags-shared',0),('RJ00000001','tags-owned',1),('RJ00000002','tags-kept',0);
 INSERT INTO work_voice_actors VALUES('RJ00000001','voice_actors-shared',0),('RJ00000001','voice_actors-owned',1),('RJ00000002','voice_actors-kept',0);
 INSERT INTO work_user_data(work_id,rating) VALUES('RJ00000002',1)`,
	)
	svc := catalogCleanup(t, repo)
	sources := []cleanup.Category{cleanup.SourceTags, cleanup.SourceVoiceActors, cleanup.SourceCircles}
	preview, err := svc.Preview(t.Context(), sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range sources {
		if preview.Candidates[category] != 0 {
			t.Fatalf("without works: %+v", preview)
		}
	}
	preview, err = svc.Preview(t.Context(), append(sources, cleanup.Works))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Candidates[cleanup.Works] != 1 {
		t.Fatalf("works: %+v", preview)
	}
	for _, category := range sources {
		if preview.Candidates[category] != 1 {
			t.Fatalf("projected sources: %+v", preview)
		}
	}
	if _, found, readErr := repo.GetWork(deletionWork); readErr != nil || !found {
		t.Fatalf("preview mutated catalog: %v %v", found, readErr)
	}
	notifications := 0
	repo.OnCatalogChanged = func() { notifications++ }
	run := cleanup.Run{ID: "catalog", Status: "running", Counts: make(map[cleanup.Category]cleanup.Count)}
	if createErr := repo.CreateCleanupRun(t.Context(), run); createErr != nil {
		t.Fatal(createErr)
	}
	if batchErr := repo.CleanupBatch(
		t.Context(),
		run.ID,
		cleanup.Works,
		time.Now(),
		[]string{string(deletionWork)},
	); batchErr != nil {
		t.Fatal(batchErr)
	}
	for _, category := range sources {
		ids, candidateErr := repo.CleanupCandidates(t.Context(), category, time.Now(), nil)
		if candidateErr != nil || len(ids) != 1 {
			t.Fatalf("candidates %s: %v %v", category, ids, candidateErr)
		}
		if batchErr := repo.CleanupBatch(t.Context(), run.ID, category, time.Now(), ids); batchErr != nil {
			t.Fatal(batchErr)
		}
	}
	if notifications != 4 {
		t.Fatalf("catalog notifications=%d, want one per committed stage", notifications)
	}
	saved, err := repo.ReadCleanupRun(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range append(sources, cleanup.Works) {
		if saved.Counts[category].Deleted != 1 {
			t.Fatalf("counts: %+v", saved.Counts)
		}
	}
}
