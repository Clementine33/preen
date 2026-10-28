package refresh_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/coverhttp"
	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/adapters/scraper"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/works/refresh"

	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/workset"
)

type worksetTransport struct{ server *httptest.Server }

func (t worksetTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = t.server.Listener.Addr().String()
	return t.server.Client().Transport.RoundTrip(clone)
}
func TestWorksetRefreshCommitsMetadataAndQueuesCoversWithoutWaitingForCDN(t *testing.T) {
	svc, cover, cat, root, names, libID, statics := worksetFixture(t)
	enableCoverDownload(t, svc)
	record, err := svc.CreateCurrentWorkset(
		t.Context(),
		workset.CreateCurrentRequest{LibraryID: libID, OperationType: refresh.Kind, FolderPaths: names},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := record.Workset.WorksetID
	gen, err := svc.StartGeneration(
		t.Context(),
		id,
		refresh.Kind,
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	defer stopDispatcher(t, svc)
	generation := waitGeneration(t, svc, id, gen.Generation.GenerationID)
	if statics.Load() != 0 {
		t.Fatal("plan performed upstream work")
	}
	op, err := svc.GetOperation(t.Context(), id, refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := svc.StartExecution(
		t.Context(),
		id,
		refresh.Kind,
		generation.RevisionID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "run"},
	)
	if err != nil {
		t.Fatal(err)
	}
	view := waitExecution(t, svc, id, execution.Execution.ExecutionID)
	if view.Status != "succeeded" || len(view.TaskResult) != 2 || statics.Load() != 1 {
		t.Fatalf("batch: %+v, requests %d", view, statics.Load())
	}
	detail, e := cat.Detail("RJ00000001")
	if e != nil || detail.Title == nil || *detail.Title != "fresh" {
		t.Fatalf("metadata missing: %+v %v", detail, e)
	}
	report, err := cover.Report(t.Context(), view.ExecutionID)
	if err != nil || report.Counts["pending"] != 2 {
		t.Fatalf("cover queue missing: %+v %v", report, err)
	}
	verifyCoverHTTP(t, svc, cover, id, view.ExecutionID)
	for _, name := range names {
		if _, err = os.Stat(filepath.Join(root, name, "cover.webp")); !os.IsNotExist(err) {
			t.Fatal("execution waited for or wrote CDN image")
		}
	}
}
func waitGeneration(t *testing.T, svc workset.Service, id, gen string) *workset.GenerationView {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		v, err := svc.GetGeneration(context.Background(), id, refresh.Kind, gen)
		if err != nil {
			t.Fatal(err)
		}
		if v.Status == "completed" {
			return v
		}
		if v.Status == "failed" {
			t.Fatalf("generation: %+v", v)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("generation timeout")
	return nil
}
func waitExecution(t *testing.T, svc workset.Service, id, execution string) *workset.ExecutionView {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		v, err := svc.GetExecution(context.Background(), id, refresh.Kind, execution, workset.ExecutionPage{})
		if err != nil {
			t.Fatal(err)
		}
		if v.Status != "queued" && v.Status != "running" {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("execution timeout")
	return nil
}

func verifyCoverHTTP(t *testing.T, svc workset.Service, cover *coverdelivery.Service, id, executionID string) {
	t.Helper()

	handler := httpapi.NewServer(httpapi.Dependencies{Token: "secret", WorksetService: svc, Covers: cover})
	path := "/api/v1/worksets/" + id + "/operations/metadata_refresh/executions/" + executionID + "/covers"
	request := httptest.NewRequest(http.MethodGet, path, nil)
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, request)
	if out.Code != 401 {
		t.Fatalf("cover report without auth: %d", out.Code)
	}
	request.Header.Set("Authorization", "Bearer secret")
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, request)
	if out.Code != 200 {
		t.Fatalf("cover report: %d %s", out.Code, out.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, path+"/cancel", nil)
	request.Header.Set("Authorization", "Bearer secret")
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, request)
	if out.Code != 200 {
		t.Fatalf("cancel: %d %s", out.Code, out.Body.String())
	}
	report, err := cover.Report(t.Context(), executionID)
	if err != nil || report.Counts["canceled"] != 2 {
		t.Fatalf("cover cancel failed: %+v %v", report, err)
	}
	request = httptest.NewRequest(http.MethodPost, path+"/retry", nil)
	request.Header.Set("Authorization", "Bearer secret")
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, request)
	if out.Code != 200 {
		t.Fatalf("retry: %d %s", out.Code, out.Body.String())
	}
	report, err = cover.Report(t.Context(), executionID)
	if err != nil || report.Counts["pending"] != 2 {
		t.Fatalf("retry report: %+v %v", report, err)
	}
	request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/worksets/other/operations/metadata_refresh/executions/"+executionID+"/covers",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret")
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, request)
	if out.Code != 404 {
		t.Fatalf("foreign execution exposed: %d", out.Code)
	}
}

func worksetFixture(
	t *testing.T,
	override ...workset.BatchExecutor,
) (workset.Service, *coverdelivery.Service, *works.Service, string, []string, string, *atomic.Int32) {
	t.Helper()
	return worksetFixtureWithRefresh(t, nil, override...)
}

func worksetFixtureWithRefresh(
	t *testing.T,
	onRefresh func(context.Context, string, []string) error,
	override ...workset.BatchExecutor,
) (workset.Service, *coverdelivery.Service, *works.Service, string, []string, string, *atomic.Int32) {
	t.Helper()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	root := t.TempDir()
	lib, err := repo.CreateLibrary("test", root)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"RJ00000001 a", "RJ00000001 b"}
	for _, name := range names {
		if err = os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	if _, err = pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(lib.ID)); err != nil {
		t.Fatal(err)
	}
	source, statics := worksetSource(t)
	cat := works.NewService(repo, nil, nil, nil, "")
	meta := refresh.NewService(cat, repo, source, source)
	gate := admission.NewGate(repo.HasActiveSession)
	cover, err := coverdelivery.New(
		coverdelivery.Dependencies{
			Store:           repo,
			Downloader:      coverhttp.New(nil),
			Source:          source,
			Files:           filesystem.CoverFiles{},
			Acquire:         gate.BeginManual,
			RefreshSelected: onRefresh,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	task := refresh.NewTask(repo, filesystem.CheckWorkDirectory)
	executor := refresh.Executor{Metadata: meta, Covers: cover}
	run := executor.Run
	if len(override) > 0 {
		run = override[0]
	}
	svc := workset.NewService(
		repo,
		1,
		1,
		[]workset.Task{task},
		func(context.Context, string, string) error { t.Error("metadata entered member scan"); return nil },
		gate.Enqueue,
		map[string]workset.BatchExecutor{refresh.Kind: run},
	)
	return svc, cover, cat, root, names, lib.ID, statics
}

func worksetSource(t *testing.T) (*scraper.Client, *atomic.Int32) {
	t.Helper()
	statics := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/maniax/api/=/product.json" {
			statics.Add(1)
			_, _ = w.Write(
				[]byte(
					`[{"workno":"RJ000001","product_name":"fresh","genres":[],"creaters":{},"language_editions":[],"image_main":{"url":"//img.dlsite.jp/main.webp"}}]`,
				),
			)
			return
		}
		_, _ = w.Write([]byte(`{"RJ000001":{"dl_count":1,"translation_info":{"is_original":true}}}`))
	}))
	t.Cleanup(server.Close)
	source := scraper.New(&http.Client{Transport: worksetTransport{server}})
	return source, statics
}

func TestMetadataSSEIncludesResultsThatFinishOutOfOrder(t *testing.T) {
	begin, proceed := make(chan struct{}), make(chan struct{})
	run := func(ctx context.Context, in workset.BatchExecutionInput) error {
		select {
		case <-begin:
		case <-ctx.Done():
			return ctx.Err()
		}
		completed := 1
		if err := in.Progress(
			in.Units[1].Index,
			workset.UnitResult{TaskResult: []byte(`{"rj":"second"}`), CompletedOperations: &completed},
		); err != nil {
			return err
		}
		select {
		case <-proceed:
		case <-ctx.Done():
			return ctx.Err()
		}
		return in.Progress(in.Units[0].Index, workset.UnitResult{
			TaskResult: []byte(`{"rj":"first"}`), CompletedOperations: &completed,
		})
	}
	svc, _, _, _, names, libID, _ := worksetFixture(t, run)
	record, err := svc.CreateCurrentWorkset(
		t.Context(),
		workset.CreateCurrentRequest{LibraryID: libID, OperationType: refresh.Kind, FolderPaths: names},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := record.Workset.WorksetID
	gen, err := svc.StartGeneration(
		t.Context(), id, refresh.Kind,
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "sse-plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	defer stopDispatcher(t, svc)
	plan := waitGeneration(t, svc, id, gen.Generation.GenerationID)
	op, err := svc.GetOperation(t.Context(), id, refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := svc.StartExecution(
		t.Context(), id, refresh.Kind, plan.RevisionID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "sse-run"},
	)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	var progress struct {
		Status              string `json:"status"`
		CompletedComponents int    `json:"completed_components"`
		CompletedOperations int    `json:"completed_operations"`
	}
	terminal := false
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err = svc.SubscribeExecution(
		ctx,
		id,
		refresh.Kind,
		execution.Execution.ExecutionID,
		func(name string, data any) error {
			switch name {
			case workset.ExecutionEventProgress:
				raw, marshalErr := json.Marshal(data)
				if marshalErr != nil {
					return marshalErr
				}
				return json.Unmarshal(raw, &progress)
			case workset.ExecutionEventSucceeded:
				terminal = true
				if progress.Status != workset.ExecStatusSucceeded ||
					progress.CompletedComponents != 2 || progress.CompletedOperations != 2 {
					t.Fatalf("terminal arrived with stale progress: %+v, want succeeded with 2/2", progress)
				}
			case workset.ExecutionEventSnapshot:
				close(begin)
			case workset.ExecutionEventComponent:
				if entry, ok := data.(workset.ExecutionComponentView); ok {
					seen[entry.ComponentIndex] = true
					if len(seen) == 1 {
						close(proceed)
					}
				}
			}
			return nil
		},
	)
	if err != nil || len(seen) != 2 || !terminal {
		t.Fatalf("out-of-order component omitted: %v %v", seen, err)
	}
}

// enableCoverDownload saves Task settings that download covers (off by
// default), so a record created afterwards seeds its draft with them.
func enableCoverDownload(t *testing.T, svc workset.Service) {
	t.Helper()
	current, err := svc.GetTaskSettings(t.Context(), refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"mode":"fill_missing","locale":"ja-jp","download_cover":true}`
	if _, err = svc.SaveTaskSettings(t.Context(), refresh.Kind, []byte(payload), current.ETag); err != nil {
		t.Fatal(err)
	}
}

// publishCommon saves common as the Task settings, the only write path of
// them, and returns the record's draft as it stands afterwards.
func publishCommon(t *testing.T, svc workset.Service, worksetID string, common refresh.Common) *workset.Draft {
	t.Helper()
	payload, err := json.Marshal(common)
	if err != nil {
		t.Fatal(err)
	}
	current, err := svc.GetTaskSettings(t.Context(), refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SaveTaskSettings(t.Context(), refresh.Kind, payload, current.ETag); err != nil {
		t.Fatal(err)
	}
	draft, err := svc.GetDraft(t.Context(), worksetID, refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func TestRefreshAllCanRunAgainWithoutChangingSettingsOrFolders(t *testing.T) {
	svc, _, _, _, names, libID, statics := worksetFixture(t)
	record, err := svc.CreateCurrentWorkset(t.Context(), workset.CreateCurrentRequest{
		LibraryID: libID, OperationType: refresh.Kind, FolderPaths: names,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := record.Workset.WorksetID
	draft, err := svc.GetDraft(t.Context(), id, refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	var document refresh.Draft
	if err = json.Unmarshal(draft.Document, &document); err != nil {
		t.Fatal(err)
	}
	document.Common.Mode = refresh.RefreshAll
	document.Common.DownloadCover = false
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	synced := publishCommon(t, svc, id, document.Common)
	op, err := svc.SaveDraft(t.Context(), id, refresh.Kind, workset.SaveDraftRequest{
		Document: raw, IfMatchVersion: synced.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	defer stopDispatcher(t, svc)
	previousPlan := ""
	for _, key := range []string{"first", "second"} {
		gen, e := svc.StartGeneration(t.Context(), id, refresh.Kind, workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "plan-" + key,
		})
		if e != nil {
			t.Fatal(e)
		}
		if !gen.Created || gen.Generation == nil {
			t.Fatalf("%s generation reused the executed plan instead of creating a fresh plan: %+v", key, gen)
		}
		plan := waitGeneration(t, svc, id, gen.Generation.GenerationID)
		if plan.RevisionID == previousPlan {
			t.Fatal("repeated refresh kept the old plan")
		}
		op, e = svc.GetOperation(t.Context(), id, refresh.Kind)
		if e != nil {
			t.Fatal(e)
		}
		execution, e := svc.StartExecution(
			t.Context(), id, refresh.Kind, plan.RevisionID,
			workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "run-" + key},
		)
		if e != nil {
			t.Fatal(e)
		}
		result := waitExecution(t, svc, id, execution.Execution.ExecutionID)
		if result.Status != "succeeded" {
			t.Fatalf("refresh failed: %+v", result)
		}
		previousPlan = plan.RevisionID
		op, e = svc.GetOperation(t.Context(), id, refresh.Kind)
		if e != nil {
			t.Fatal(e)
		}
		replay, e := svc.StartGeneration(t.Context(), id, refresh.Kind, workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "plan-" + key,
		})
		if e != nil {
			t.Fatal(e)
		}
		if replay.Created || replay.Generation == nil || replay.Generation.GenerationID != gen.Generation.GenerationID {
			t.Fatalf("a retry with the original key created new work: %+v", replay)
		}
	}
	if statics.Load() != 2 {
		t.Fatalf("the second execution did not fetch fresh metadata: %d static requests", statics.Load())
	}
}

func stopDispatcher(t *testing.T, svc workset.Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := svc.DispatcherHandle().Stop(ctx); err != nil {
		t.Error(err)
	}
}
