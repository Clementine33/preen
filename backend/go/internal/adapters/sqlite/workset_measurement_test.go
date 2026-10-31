//nolint:testpackage // The opt-in harness instruments the repository's private connection pools.
package sqlite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/librarypath"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// TestMeasureWorksetLifecycle is deliberately opt-in and uses disposable files.
// Run: PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=500 PREEN_MEASURE_AUDIO=12 \
// go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=30m
// Network and media tools are simulated; SQL, scanning, admission, planning,
// execution, file publication, inventory synchronization and HTTP reads are real.
func TestMeasureWorksetLifecycle(t *testing.T) {
	if os.Getenv("PREEN_MEASURE_WORKSET") != "1" {
		t.Skip("set PREEN_MEASURE_WORKSET=1 to run synthetic lifecycle measurements")
	}
	members := worksetMeasurementInt(t, "PREEN_MEASURE_MEMBERS", 500)
	audio := worksetMeasurementInt(t, "PREEN_MEASURE_AUDIO", 12)
	t.Logf(
		"load members=%d audio_per_member=%d audio_total=%d os=%s arch=%s go=%s cpus=%d gomaxprocs=%d temp=%s",
		members,
		audio,
		members*audio,
		runtime.GOOS,
		runtime.GOARCH,
		runtime.Version(),
		runtime.NumCPU(),
		runtime.GOMAXPROCS(0),
		os.TempDir(),
	)
	f := newWorksetMeasurement(t, members, audio)
	defer f.sql.report(t)
	id := f.overviewEntry(t)
	var renamePlan string
	// Both generations replace fully executed plans. Execute the replacement plans
	// too, so replacing the record also removes nonempty execution/cover results.
	for round := range 2 {
		f.source.version.Store(uint32(round + 1))
		conversionPlan, done := f.measureConversionRound(t, id, round)
		if done {
			return
		}
		metadataPlan := f.generate(t, id, refresh.Kind, round)
		if round == 0 && os.Getenv("PREEN_MEASURE_BROWSER") == "1" {
			f.browser(t, "task-plan", id, metadataPlan, "", "", refresh.Kind)
			f.executeSampled(t, id, refresh.Kind, metadataPlan, round)
		} else {
			f.execute(t, id, refresh.Kind, metadataPlan, round)
		}
		f.deliver(t, id, round)
		f.read(
			t,
			"revision/metadata_refresh",
			"/api/v1/worksets/"+id+"/operations/metadata_refresh/revisions/"+metadataPlan,
		)
		// Rename is only planned: its current plan makes all three operations
		// carry one when the shared records are read.
		renamePlan = f.generate(t, id, "rename", round)
		if round == 0 && os.Getenv("PREEN_MEASURE_BROWSER") == "1" {
			f.browser(t, "task-plan", id, renamePlan, "", "", "rename")
		}
		f.read(t, "revision/rename", "/api/v1/worksets/"+id+"/operations/rename/revisions/"+renamePlan)
		f.read(t, "current", "/api/v1/libraries/"+f.libraryID+"/operations/conversion/current")
		f.read(t, "operation", "/api/v1/worksets/"+id+"/operations/conversion")
		f.read(t, "workset", "/api/v1/worksets/"+id)
		f.read(
			t,
			"revision/conversion/three_plans",
			"/api/v1/worksets/"+id+"/operations/conversion/revisions/"+conversionPlan,
		)
		// Same navigation as the single-plan sample, now with all three plans current.
		if round == 0 && os.Getenv("PREEN_MEASURE_KEPT_SNAPSHOT") != "1" {
			f.browser(t, "revision", id, conversionPlan, f.readLargestMember(t, id, conversionPlan), "")
		}
	}
	if os.Getenv("PREEN_MEASURE_BROWSER") == "1" {
		// Rename is executed last: it moves the member folders the earlier
		// rounds address by name.
		f.executeSampled(t, id, "rename", renamePlan, 1)
	}
	replacement := f.create(t, id)
	if replacement == id {
		t.Fatal("record was not replaced")
	}
	if current, err := f.svc.GetCurrentWorkset(
		t.Context(),
		f.libraryID,
		workset.OperationTypeConversion,
	); err != nil || current == nil ||
		current.WorksetID != replacement {
		t.Fatalf("replacement current=%+v err=%v", current, err)
	}
	// Drain independently, as the process worker does; driver timing reports each batch.
	var batches int
	var deleted int64
	for {
		n, err := f.repo.DeleteRetiredPlanBatch(t.Context(), 1000)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		batches++
		deleted += n
	}
	t.Logf("retired cleanup batches=%d rows=%d", batches, deleted)
}

func worksetMeasurementInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	if value := os.Getenv(key); value != "" {
		number, err := strconv.Atoi(value)
		if err != nil || number < 1 {
			t.Fatalf("%s must be a positive integer", key)
		}
		return number
	}
	return fallback
}

type worksetMeasurement struct {
	repo            *Repository
	svc             workset.Service
	covers          *coverdelivery.Service
	sql             *worksetSQLMeasurement
	handler         http.Handler
	root, libraryID string
	names           []string
	tasks           []workset.Task
	source          worksetMeasurementSource
}

func (f *worksetMeasurement) create(t *testing.T, expected string) string {
	t.Helper()
	if len(f.names) <= workset.MaxMembers {
		result, err := f.svc.CreateCurrentWorkset(t.Context(), workset.CreateCurrentRequest{
			LibraryID:         f.libraryID,
			OperationType:     workset.OperationTypeConversion,
			Selection:         workset.Selection{All: true},
			ExpectedCurrentID: expected,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.Workset.WorksetID
	}
	// Baseline-only bypass of admission's current member cap. Use the same atomic
	// repository write with task-seeded drafts; never change the production limit.
	t.Logf("baseline admission bypass members=%d production_cap=%d", len(f.names), workset.MaxMembers)
	now := time.Now()
	id := fmt.Sprintf("measure-%d", now.UnixNano())
	row := &workset.Workset{
		ID:          id,
		Title:       "measurement",
		LibraryID:   f.libraryID,
		RootPath:    f.root,
		RootPathKey: librarypath.RootPathKey(f.root),
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	members := make([]workset.WorksetMember, len(f.names))
	for i, name := range f.names {
		members[i] = workset.WorksetMember{
			WorksetID:   id,
			MemberID:    fmt.Sprintf("%s-m%d", id, i),
			MemberIndex: i,
			RelPath:     name,
			FolderName:  name,
			FolderPath:  filepath.Join(f.root, name),
		}
	}
	var operations []workset.Operation
	var drafts []workset.OperationDraft
	for _, task := range f.tasks {
		var common []byte
		if task.Kind() == refresh.Kind {
			common = []byte(`{"mode":"refresh_all","locale":"ja-jp","download_cover":true}`)
		}
		raw, hash, schema := task.SeedDraft(common)
		operations = append(
			operations,
			workset.Operation{WorksetID: id, OperationType: task.Kind(), Version: 1, CreatedAt: now, UpdatedAt: now},
		)
		drafts = append(
			drafts,
			workset.OperationDraft{
				WorksetID:     id,
				OperationType: task.Kind(),
				SchemaVersion: schema,
				DraftJSON:     string(raw),
				DraftHash:     hash,
				UpdatedAt:     now,
			},
		)
	}
	if err := f.repo.ReplaceCurrentWorkset(row, members, operations, drafts, expected); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *worksetMeasurement) generate(t *testing.T, id, kind string, round int) string {
	t.Helper()
	if kind == workset.OperationTypeConversion {
		f.excludeBeyondParticipationLimit(t, id)
	}
	op, err := f.svc.GetOperation(t.Context(), id, kind)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	result, err := f.svc.StartGeneration(
		t.Context(),
		id,
		kind,
		workset.StartGenerationRequest{
			IfMatchVersion: op.Version,
			IdempotencyKey: fmt.Sprintf("%s-plan-%d", kind, round),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	// An unexecuted rename plan with unchanged inputs is replayed, not replaced.
	if !result.Created && kind == "rename" && result.Revision != nil {
		return result.Revision.PlanID
	}
	if !result.Created || result.Generation == nil {
		t.Fatalf("generation reused instead of replacing executed plan: %+v", result)
	}
	var revision string
	worksetMeasurementWait(t, func() bool {
		view, err := f.svc.GetGeneration(t.Context(), id, kind, result.Generation.GenerationID)
		if err != nil {
			t.Fatal(err)
		}
		switch view.Status {
		case "completed":
			revision = view.RevisionID
			return true
		case "queued", "running":
			return false
		default:
			t.Fatalf("generation failed: %+v", view)
		}
		return false
	})
	t.Logf(
		"generation kind=%s round=%d end_to_end_ms=%.3f",
		kind,
		round,
		float64(time.Since(started))/float64(time.Millisecond),
	)
	return revision
}

func (f *worksetMeasurement) execute(t *testing.T, id, kind, plan string, round int) string {
	t.Helper()
	return f.awaitExecution(t, kind, round, f.startExecution(t, id, kind, plan, round))
}

func (f *worksetMeasurement) startExecution(t *testing.T, id, kind, plan string, round int) string {
	t.Helper()
	op, err := f.svc.GetOperation(t.Context(), id, kind)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.svc.StartExecution(
		t.Context(),
		id,
		kind,
		plan,
		workset.StartExecutionRequest{
			IfMatchVersion: op.Version,
			IdempotencyKey: fmt.Sprintf("%s-execute-%d", kind, round),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result.Execution.ExecutionID
}

func (f *worksetMeasurement) awaitExecution(t *testing.T, kind string, round int, executionID string) string {
	t.Helper()
	worksetMeasurementWait(t, func() bool {
		view, err := f.repo.GetExecution(executionID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status == "queued" || view.Status == "running" {
			return false
		}
		if view.Status != "succeeded" || view.TotalComponents == 0 || view.CompletedComponents != view.TotalComponents {
			t.Fatalf("execution failed: status=%s code=%s message=%s", view.Status, view.ErrorCode, view.ErrorMessage)
		}
		return true
	})
	var requestBytes, components int
	if scanErr := f.repo.DB().QueryRow(
		`SELECT length(q.request_json), e.total_components FROM plan_executions e
		 JOIN plan_execution_requests q ON q.execution_id = e.execution_id WHERE e.execution_id = ?`,
		executionID,
	).Scan(&requestBytes, &components); scanErr != nil {
		t.Fatal(scanErr)
	}
	t.Logf("execution kind=%s round=%d id=%s status=succeeded request_bytes=%d components=%d",
		kind, round, executionID, requestBytes, components)
	return executionID
}

func (f *worksetMeasurement) deliver(t *testing.T, id string, round int) {
	t.Helper()
	op, err := f.svc.GetOperation(t.Context(), id, refresh.Kind)
	if err != nil || op.LatestExecution == nil {
		t.Fatalf("metadata execution: %+v %v", op, err)
	}
	executionID := op.LatestExecution.ExecutionID
	report, err := f.covers.Report(t.Context(), executionID)
	if err != nil || report.Counts["pending"] != len(f.names) {
		t.Fatalf("missing pending covers: %+v %v", report, err)
	}
	for {
		processed, deliveryErr := f.covers.ProcessNext(t.Context())
		if deliveryErr != nil {
			t.Fatal(deliveryErr)
		}
		if !processed {
			break
		}
	}
	report, err = f.covers.Report(t.Context(), executionID)
	if err != nil || report.Counts["succeeded"] != len(f.names) {
		t.Fatalf("covers did not publish: %+v %v", report, err)
	}
	for _, name := range f.names {
		if _, err := os.Stat(filepath.Join(f.root, name, "cover.png")); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("covers round=%d succeeded=%d", round, report.Counts["succeeded"])
}

func (f *worksetMeasurement) read(t *testing.T, label, path string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	started := time.Now()
	f.handler.ServeHTTP(response, request)
	elapsed := time.Since(started)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
	}
	t.Logf(
		"http_read endpoint=%s elapsed_ms=%.3f bytes=%d",
		label,
		float64(elapsed)/float64(time.Millisecond),
		response.Body.Len(),
	)
}

func worksetMeasurementWait(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Minute)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("measurement session timed out")
		}
		time.Sleep(10 * time.Millisecond) // Real disk and SQLite workers cannot use synctest.
	}
}

// excludeBeyondParticipationLimit excludes the members past conversion's
// Participation Limit, as a user must, so oversized fixtures still plan.
func (f *worksetMeasurement) excludeBeyondParticipationLimit(t *testing.T, id string) {
	t.Helper()
	view, err := f.repo.ListWorksetMembers(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(view) <= conversion.MaxParticipants {
		return
	}
	draft, err := f.svc.GetDraft(t.Context(), id, workset.OperationTypeConversion)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := conversion.ParseDraft(string(draft.Document))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Members) == len(view)-conversion.MaxParticipants {
		return
	}
	doc.Members = doc.Members[:0]
	for _, member := range view[conversion.MaxParticipants:] {
		doc.Members = append(doc.Members, conversion.DraftMember{MemberID: member.MemberID, Excluded: true})
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	op, err := f.svc.GetOperation(t.Context(), id, workset.OperationTypeConversion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.SaveDraft(t.Context(), id, workset.OperationTypeConversion,
		workset.SaveDraftRequest{Document: raw, IfMatchVersion: op.Version}); err != nil {
		t.Fatal(err)
	}
	t.Logf("participation limit: excluded %d of %d members", len(view)-conversion.MaxParticipants, len(view))
}
