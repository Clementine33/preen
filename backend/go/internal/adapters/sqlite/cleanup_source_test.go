package sqlite_test

import (
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/works"
)

func TestCleanupSourcesRechecksReferencesAndLeavesNewCandidatesForNextRun(t *testing.T) {
	for _, tc := range []struct {
		category            cleanup.Category
		table, column, bind string
	}{
		{cleanup.SourceTags, "tags", "source_tag_id", `INSERT INTO work_tags VALUES('RJ00000001','old',0)`},
		{cleanup.SourceVoiceActors, "voice_actors", "source_voice_actor_id", `INSERT INTO work_voice_actors VALUES('RJ00000001','old',0)`},
		{cleanup.SourceCircles, "circles", "source_circle_id", `UPDATE works SET circle_id='old' WHERE id='RJ00000001'`},
	} {
		t.Run(string(tc.category), func(t *testing.T) {
			repo := deletionRepository(t)
			deletionSQL(
				t,
				repo,
				"INSERT INTO "+tc.table+"(id,source,"+tc.column+",name) VALUES('old','dlsite','1','same name')",
			)
			ids, err := repo.CleanupCandidates(t.Context(), tc.category, time.Now(), nil)
			if err != nil || len(ids) != 1 {
				t.Fatalf("candidates %v %v", ids, err)
			}
			deletionSQL(t, repo, tc.bind)
			deletionSQL(
				t,
				repo,
				"INSERT INTO "+tc.table+"(id,source,"+tc.column+",name) VALUES('new','dlsite','2','same name')",
			)
			startCatalogRun(t, repo)
			notifications := 0
			repo.OnCatalogChanged = func() { notifications++ }
			if batchErr := repo.CleanupBatch(t.Context(), "catalog", tc.category, time.Now(), ids); batchErr != nil {
				t.Fatal(batchErr)
			}
			saved, err := repo.ReadCleanupRun(t.Context())
			if err != nil || saved.Counts[tc.category] != (cleanup.Count{Checked: 1, Skipped: 1}) {
				t.Fatalf("counts %+v %v", saved, err)
			}
			var count int
			if err := repo.DB().
				QueryRow("SELECT COUNT(*) FROM " + tc.table).
				Scan(&count); err != nil || count != 2 ||
				notifications != 0 {
				t.Fatalf("sources %d %v notifications=%d", count, err, notifications)
			}
		})
	}
}

func TestCleanupKeepsRefreshedSourceTagIdentityAcrossLanguages(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(t, repo, `INSERT INTO work_user_data(work_id,rating) VALUES('RJ00000001',1)`)
	svc := catalogCleanup(t, repo)
	firstID := ""
	for i, name := range []string{"日本語タグ", "简体中文标签"} {
		now := time.Now().UTC().Add(time.Duration(i) * time.Second)
		_, err := repo.ImportMetadata(
			&works.ValidatedImport{
				Target: deletionWork,
				Core: &works.CoreGroup{
					Source:    works.SourceDLsite,
					FetchedAt: now,
					Tags:      []works.EntityInput{{SourceID: new("68"), Name: name}},
				},
			},
			now,
		)
		if err != nil {
			t.Fatal(err)
		}
		detail, found, err := repo.GetWork(deletionWork)
		if err != nil || !found || len(detail.SourceTags) != 1 || detail.SourceTags[0].Name != name {
			t.Fatalf("detail %+v %v %v", detail, found, err)
		}
		if i == 0 {
			firstID = detail.SourceTags[0].ID
		} else if detail.SourceTags[0].ID != firstID {
			t.Fatal("language refresh changed source identity")
		}
		preview, err := svc.Preview(t.Context(), []cleanup.Category{cleanup.Works, cleanup.SourceTags})
		if err != nil || preview.Candidates[cleanup.Works] != 0 || preview.Candidates[cleanup.SourceTags] != 0 {
			t.Fatalf("preview %+v %v", preview, err)
		}
	}
	startCatalogRun(t, repo)
	if batchErr := repo.CleanupBatch(
		t.Context(),
		"catalog",
		cleanup.SourceTags,
		time.Now(),
		[]string{firstID},
	); batchErr != nil {
		t.Fatal(batchErr)
	}
	detail, _, err := repo.GetWork(deletionWork)
	if err != nil || len(detail.SourceTags) != 1 || detail.SourceTags[0].ID != firstID {
		t.Fatalf("deleted refreshed tag: %+v %v", detail, err)
	}
}

func TestCleanupWorksAloneCascadesOwnedSourcesAndKeepsSharedSourcesReportsAndSettings(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(
		t,
		repo,
		`INSERT INTO work_user_data(work_id) VALUES('RJ00000001'); INSERT INTO user_tags VALUES('unused','keep');
 INSERT INTO relation_reports VALUES('RJ00000001','original',NULL,NULL,NULL,'dlsite','');
 INSERT INTO works(id,collected_at,created_at,updated_at) VALUES('RJ00000002','','','');
 INSERT INTO work_user_data(work_id,rating) VALUES('RJ00000002',1);
 INSERT INTO work_translations VALUES('RJ00000002','child',NULL,'RJ00000001','RJ00000001')`,
	)
	for _, table := range []string{"tags", "voice_actors", "circles"} {
		deletionSQL(
			t,
			repo,
			"INSERT INTO "+table+"(id,source,name,work_id) VALUES('owned','dlsite','same','RJ00000001'),('shared','dlsite','same',NULL)",
		)
	}
	if _, err := repo.AddTag("keep classifier"); err != nil {
		t.Fatal(err)
	}
	startCatalogRun(t, repo)
	notifications := 0
	repo.OnCatalogChanged = func() { notifications++ }
	if batchErr := repo.CleanupBatch(
		t.Context(),
		"catalog",
		cleanup.Works,
		time.Now(),
		[]string{string(deletionWork)},
	); batchErr != nil {
		t.Fatal(batchErr)
	}
	if notifications != 1 {
		t.Fatalf("notifications=%d", notifications)
	}
	for _, table := range []string{"tags", "voice_actors", "circles", "relation_reports", "user_tags", "classifier_tag_library"} {
		var count int
		if err := repo.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d %v", table, count, err)
		}
	}
	var parent, original string
	if err := repo.DB().
		QueryRow("SELECT parent_work_id, original_work_id FROM work_translations WHERE work_id='RJ00000002'").
		Scan(&parent, &original); err != nil || parent != "RJ00000001" ||
		original != "RJ00000001" {
		t.Fatalf("other derived relation changed: %s %s %v", parent, original, err)
	}
	saved, err := repo.ReadCleanupRun(t.Context())
	if err != nil || len(saved.Counts) != 1 || saved.Counts[cleanup.Works].Deleted != 1 {
		t.Fatalf("cascade counts %+v %v", saved, err)
	}
}
