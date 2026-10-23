package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/works"
)

func TestCleanupHTTPCatalogSelectionProtectionCountsAndMissingEdits(t *testing.T) {
	_, repo := deletionServer(t)
	for i := 2; i <= 10; i++ {
		if _, err := repo.DB().
			Exec("INSERT INTO works(id,collected_at,created_at,updated_at) VALUES(?,'','','')", fmt.Sprintf("RJ%08d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO work_user_data(work_id,rating) VALUES('RJ00000002',1)`,
		`INSERT INTO work_user_data(work_id,note) VALUES('RJ00000003','note'),('RJ00000004',' ')`,
		`INSERT INTO user_tags VALUES('keep','keep'); INSERT INTO work_user_tags VALUES('RJ00000005','keep')`,
		`INSERT INTO collection_works VALUES('favorites','RJ00000006',0)`,
		`INSERT INTO collections VALUES('keep','keep','',''); INSERT INTO collection_works VALUES('keep','RJ00000007',0)`,
		`INSERT INTO work_listening VALUES('RJ00000008','','track',0,0,1,NULL,'client',1,''),('RJ00000009','',NULL,NULL,NULL,NULL,'finished','client',1,'')`,
		`INSERT INTO libraries(id,name,root_path,created_at,updated_at) VALUES('offline','Offline','/nonexistent-preen-library','',''); INSERT INTO work_copies VALUES('copy','RJ00000010','offline','work','')`,
		`INSERT INTO tags(id,source,source_tag_id,name) VALUES('shared','dlsite','68','日本語'); INSERT INTO tags(id,source,name,work_id) VALUES('owned','dlsite','owned','RJ00000001'); INSERT INTO work_tags VALUES('RJ00000001','shared',0),('RJ00000001','owned',1)`,
		`INSERT INTO voice_actors(id,source,source_voice_actor_id,name) VALUES('shared','dlsite','1','voice'); INSERT INTO work_voice_actors VALUES('RJ00000001','shared',0)`,
		`INSERT INTO circles(id,source,source_circle_id,name) VALUES('shared','dlsite','1','circle'); UPDATE works SET circle_id='shared' WHERE id='RJ00000001'`,
	} {
		if _, err := repo.DB().Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	svc := cleanup.NewService(t.Context(), repo, admission.NewGate(nil), cleanup.Options{YieldDelay: time.Millisecond})
	t.Cleanup(svc.Shutdown)
	handler := httpapi.NewServer(
		httpapi.Dependencies{
			Cleanup:     svc,
			Works:       works.NewService(repo, nil, nil, nil, "test"),
			Collections: collections.NewService(repo),
		},
	)
	sources := `{"categories":["source_tags","source_voice_actors","source_circles"]}`
	response := deletionRequest(handler, http.MethodPost, "maintenance/cleanup/preview", sources)
	var preview cleanup.Preview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || len(preview.Candidates) != 3 || preview.Candidates[cleanup.SourceTags] != 0 ||
		preview.Candidates[cleanup.SourceVoiceActors] != 0 ||
		preview.Candidates[cleanup.SourceCircles] != 0 {
		t.Fatalf("source-only preview %d %s", response.Code, response.Body)
	}
	all := `{"categories":["source_circles","source_voice_actors","source_tags","works","works","cover_downloads","plan_generations","scan_sessions"]}`
	response = deletionRequest(handler, http.MethodPost, "maintenance/cleanup/preview", all)
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	order := []cleanup.Category{
		cleanup.ScanSessions,
		cleanup.PlanGenerations,
		cleanup.CoverDownloads,
		cleanup.Works,
		cleanup.SourceTags,
		cleanup.SourceVoiceActors,
		cleanup.SourceCircles,
	}
	if response.Code != 200 || !reflect.DeepEqual(preview.Categories, order) {
		t.Fatalf("normalized preview %d %s", response.Code, response.Body)
	}
	for _, category := range order[3:] {
		if preview.Candidates[category] != 1 {
			t.Fatalf("projected preview %d %s", response.Code, response.Body)
		}
	}
	response = deletionRequest(handler, http.MethodPost, "maintenance/cleanup", all)
	if response.Code != 202 {
		t.Fatalf("start %d %s", response.Code, response.Body)
	}
	var envelope struct {
		Run cleanup.Run `json:"run"`
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		response = deletionRequest(handler, http.MethodGet, "maintenance/cleanup", "")
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Run.Status != "running" && envelope.Run.Status != "stopping" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cleanup did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if envelope.Run.Status != "succeeded" || !reflect.DeepEqual(envelope.Run.Categories, order) {
		t.Fatalf("run %d %s", response.Code, response.Body)
	}
	for _, category := range order[3:] {
		if envelope.Run.Counts[category] != (cleanup.Count{Checked: 1, Deleted: 1}) {
			t.Fatalf("counts %s: %s", category, response.Body)
		}
	}
	assertCleanupHTTPCatalogAfterRun(t, handler)
}

func assertCleanupHTTPCatalogAfterRun(t *testing.T, handler http.Handler) {
	t.Helper()
	for i := 2; i <= 10; i++ {
		response := deletionRequest(handler, http.MethodGet, fmt.Sprintf("works/RJ%08d", i), "")
		if response.Code != 200 {
			t.Fatalf("protected work %d: %d %s", i, response.Code, response.Body)
		}
	}
	for _, req := range []struct{ method, path, body string }{
		{http.MethodPatch, "works/RJ00000001/user-data", `{"rating":8}`},
		{http.MethodPut, "works/RJ00000001/user-tags", `{"tag_ids":[]}`},
		{http.MethodPut, "works/RJ00000001/listening", `{"track_key":"track","track_count":1,"client":"client","seq":1}`},
		{http.MethodDelete, "works/RJ00000001/listening", ""},
		{http.MethodPost, "collections/favorites/works", `{"rjs":["RJ00000001"]}`},
	} {
		response := deletionRequest(handler, req.method, req.path, req.body)
		var errEnvelope struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &errEnvelope); err != nil {
			t.Fatal(err)
		}
		if response.Code != 404 || errEnvelope.Code != works.CodeWorkNotFound {
			t.Fatalf("edit after cleanup %s %s: %d %s", req.method, req.path, response.Code, response.Body)
		}
	}
}
