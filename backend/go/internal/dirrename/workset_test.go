package dirrename_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestWorksetAcceptsNoAudioRenameAndExecutesFrozenProposal(t *testing.T) {
	repo, task, member := fixture(t, "title")
	svc := workset.NewService(repo, 1, 1, []workset.Task{task}, func(context.Context, string, string) error {
		t.Error("rename entered member scan")
		return nil
	}, nil)
	libs, err := repo.ListLibraries()
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateCurrentWorkset(
		t.Context(),
		workset.CreateCurrentRequest{
			LibraryID:     libs[0].ID,
			OperationType: "rename",
			FolderPaths:   []string{member.RelPath},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := created.Workset.WorksetID
	gen, err := svc.StartGeneration(
		t.Context(),
		id,
		"rename",
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	t.Cleanup(func() {
		if stopErr := svc.DispatcherHandle().Stop(context.Background()); stopErr != nil {
			t.Error(stopErr)
		}
	})
	var finished *workset.GenerationView
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		finished, err = svc.GetGeneration(t.Context(), id, "rename", gen.Generation.GenerationID)
		if err != nil {
			t.Fatal(err)
		}
		if finished.Status == workset.GenStatusCompleted || finished.Status == workset.GenStatusFailed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if finished.Status != workset.GenStatusCompleted {
		t.Fatalf("generation: %+v", finished)
	}
	if _, reviewErr := svc.GetRevision(t.Context(), id, "rename", finished.RevisionID); reviewErr != nil {
		t.Fatal(reviewErr)
	}
	verifyHTTPReviewAndExecution(t, svc, id, finished.RevisionID)
	var executions int
	if err := repo.DB().
		QueryRow("SELECT count(*) FROM plan_executions").
		Scan(&executions); err != nil ||
		executions != 1 {
		t.Fatalf("execution records: %d, %v", executions, err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(member.FolderPath); os.IsNotExist(err) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("rename did not move source")
}

func verifyHTTPReviewAndExecution(t *testing.T, svc workset.Service, id, revision string) {
	t.Helper()
	handler := httpapi.NewServer(httpapi.Dependencies{Token: "test-token", WorksetService: svc})
	reviewRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/worksets/"+id+"/operations/rename/revisions/"+revision,
		nil,
	)
	reviewRequest.Header.Set("Authorization", "Bearer test-token")
	reviewResponse := httptest.NewRecorder()
	handler.ServeHTTP(reviewResponse, reviewRequest)
	if reviewResponse.Code != http.StatusOK {
		t.Fatalf("review HTTP: %d, %s", reviewResponse.Code, reviewResponse.Body.String())
	}
	var wire struct {
		Task struct {
			Kind    string `json:"kind"`
			Payload struct {
				Components []json.RawMessage `json:"components"`
			} `json:"payload"`
		} `json:"task"`
		Members []struct {
			Conclusion workset.MemberConclusion `json:"conclusion"`
		} `json:"members"`
	}
	if decodeErr := json.Unmarshal(reviewResponse.Body.Bytes(), &wire); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if wire.Task.Kind != "rename" || len(wire.Task.Payload.Components) != 1 ||
		!bytes.Contains(wire.Task.Payload.Components[0], []byte(`"target_name":"[RJ00000001] title"`)) {
		t.Fatalf("review payload: %s", reviewResponse.Body.String())
	}
	if len(wire.Members) != 1 || !bytes.Equal(wire.Members[0].Conclusion.Task, wire.Task.Payload.Components[0]) {
		t.Fatalf("member conclusion differs from frozen component: %s", reviewResponse.Body.String())
	}
	op, err := svc.GetOperation(t.Context(), id, "rename")
	if err != nil {
		t.Fatal(err)
	}
	executionRequest := httptest.NewRequest(http.MethodPost, reviewRequest.URL.Path+"/executions", nil)
	executionRequest.Header.Set("Authorization", "Bearer test-token")
	executionRequest.Header.Set("If-Match", strconv.Itoa(op.Version))
	executionRequest.Header.Set("Idempotency-Key", "http-execute")
	executionResponse := httptest.NewRecorder()
	handler.ServeHTTP(executionResponse, executionRequest)
	if executionResponse.Code != http.StatusAccepted {
		t.Fatalf("execute HTTP: %d, %s", executionResponse.Code, executionResponse.Body.String())
	}
}

func TestMissingSourceIsReportedOnceInReview(t *testing.T) {
	repo, task, member := fixture(t, "title")
	if err := os.Remove(member.FolderPath); err != nil {
		t.Fatal(err)
	}
	draft, _, _ := task.SeedDraft(nil)
	snapshot, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{
			LibraryRoot: filepath.ToSlash(filepath.Dir(filepath.FromSlash(member.FolderPath))),
			RawDraft:    draft,
			Members:     []*workset.WorksetMember{member},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Units) != 1 || snapshot.Units[0].ReasonCode != "SOURCE_MISSING" ||
		snapshot.Roots[0].Status != "missing" {
		t.Fatalf("missing root: %+v", snapshot)
	}
	svc := workset.NewService(repo, 1, 1, []workset.Task{task}, nil, nil)
	libs, err := repo.ListLibraries()
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateCurrentWorkset(
		t.Context(),
		workset.CreateCurrentRequest{
			LibraryID:     libs[0].ID,
			OperationType: "rename",
			FolderPaths:   []string{member.RelPath},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := created.Workset.WorksetID
	gen, err := svc.StartGeneration(
		t.Context(),
		id,
		"rename",
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "missing"},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	t.Cleanup(func() {
		if stopErr := svc.DispatcherHandle().Stop(context.Background()); stopErr != nil {
			t.Error(stopErr)
		}
	})
	var finished *workset.GenerationView
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		finished, err = svc.GetGeneration(t.Context(), id, "rename", gen.Generation.GenerationID)
		if err != nil {
			t.Fatal(err)
		}
		if finished.Status == workset.GenStatusCompleted || finished.Status == workset.GenStatusFailed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if finished.Status != workset.GenStatusCompleted {
		t.Fatalf("generation: %+v", finished)
	}
	review, err := svc.GetRevision(t.Context(), id, "rename", finished.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if review.Counts.Blocked != 1 || review.Counts.Unchanged != 0 {
		t.Fatalf("one missing member counted twice: %+v", review.Counts)
	}
}

func TestDraftPutPassesTheRenamePayloadToTheTask(t *testing.T) {
	repo, task, member := fixture(t, "title")
	svc := workset.NewService(repo, 1, 1, []workset.Task{task}, nil, nil)
	libs, err := repo.ListLibraries()
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateCurrentWorkset(t.Context(), workset.CreateCurrentRequest{
		LibraryID: libs[0].ID, OperationType: "rename", FolderPaths: []string{member.RelPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Workset.WorksetID
	handler := httpapi.NewServer(httpapi.Dependencies{Token: "test-token", WorksetService: svc})
	put := func(version int, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/worksets/"+id+"/operations/rename/draft",
			strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer test-token")
		request.Header.Set("If-Match", strconv.Itoa(version))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	// The draft PUT keeps the document shape; its common section must equal the
	// Task settings (here: the record's own, as none were saved).
	seeded, err := svc.GetDraft(t.Context(), id, "rename")
	if err != nil {
		t.Fatal(err)
	}
	saved := put(seeded.Version, string(seeded.Document))
	if saved.Code != http.StatusOK {
		t.Fatalf("save: %d, %s", saved.Code, saved.Body.String())
	}
	draft, err := svc.GetDraft(t.Context(), id, "rename")
	if err != nil {
		t.Fatal(err)
	}
	if string(draft.Document) != string(seeded.Document) {
		t.Fatalf("stored draft: %s", draft.Document)
	}

	// Another template is the settings endpoint's to write.
	changed := put(draft.Version, `{"schema_version":1,"common":{"template":"{rj} x"},"members":[]}`)
	if changed.Code != http.StatusConflict || !strings.Contains(changed.Body.String(), "COMMON_SETTINGS_MISMATCH") {
		t.Fatalf("changed common section: %d, %s", changed.Code, changed.Body.String())
	}

	// Validation stays the task's: an unknown field is its INVALID_DRAFT.
	refused := put(draft.Version, `{"schema_version":1,"common":{"template":"{rj}"},"members":[],"extra":1}`)
	if refused.Code != http.StatusBadRequest || !strings.Contains(refused.Body.String(), "INVALID_DRAFT") {
		t.Fatalf("unknown field: %d, %s", refused.Code, refused.Body.String())
	}
}
