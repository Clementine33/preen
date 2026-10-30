//nolint:testpackage // The opt-in harness uses its real repository and handler fixture.
package sqlite

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func (f *worksetMeasurement) readLargestMember(t *testing.T, id, plan string) string {
	t.Helper()
	var memberID, root string
	var components int
	err := f.repo.db.QueryRow(`SELECT r.root_path, COUNT(c.component_index)
 FROM plan_roots r JOIN conversion_components c
 ON c.plan_id = r.plan_id AND c.root_index = r.root_index
 WHERE r.plan_id = ? GROUP BY r.root_index
 ORDER BY COUNT(c.component_index) DESC, r.root_index LIMIT 1`, plan).Scan(&root, &components)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.db.QueryRow(`SELECT member_id FROM workset_members
 WHERE workset_id = ? AND folder_path = ? ORDER BY member_index LIMIT 1`, id, root).Scan(&memberID); err != nil {
		t.Fatal(err)
	}
	t.Logf("largest_member member_id=%s components=%d", memberID, components)
	f.read(
		t,
		"revision/conversion/member",
		"/api/v1/worksets/"+id+"/operations/conversion/revisions/"+plan+"/members/"+memberID,
	)
	return memberID
}

// browser makes the exact lifecycle fixture available over loopback. Only the
// optional measurement process owns the server; no production listener changes.
func (f *worksetMeasurement) browser(t *testing.T, phase, id, plan, member, execution string) {
	t.Helper()
	if os.Getenv("PREEN_MEASURE_BROWSER") != "1" {
		return
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		measured := &worksetMeasuredResponse{ResponseWriter: w, started: time.Now(), status: http.StatusOK}
		f.handler.ServeHTTP(measured, r)
	}))
	defer server.Close()
	script, err := filepath.Abs("../../../../../frontend/web/e2e/measure-workset-reads.mjs")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(
		t.Context(),
		"node",
		script,
		phase,
		server.URL,
		f.libraryID,
		id,
		plan,
		member,
		execution,
	)
	output, err := command.CombinedOutput()
	t.Logf("browser phase=%s\n%s", phase, output)
	if err != nil {
		t.Fatal(err)
	}
}

type worksetMeasuredResponse struct {
	http.ResponseWriter

	started time.Time
	status  int
	wrote   bool
}

// Delay committing headers until the first write (SSE also does not flush
// before its first event). Timing ends before the real socket write and includes
// the production handler's query, view and JSON/SSE encoding.
func (w *worksetMeasuredResponse) WriteHeader(status int) { w.status = status }
func (w *worksetMeasuredResponse) Write(data []byte) (int, error) {
	if !w.wrote {
		w.Header().
			Set("Server-Timing", fmt.Sprintf("handler;dur=%.3f", float64(time.Since(w.started))/float64(time.Millisecond)))
		w.ResponseWriter.WriteHeader(w.status)
		w.wrote = true
	}
	return w.ResponseWriter.Write(data)
}
func (w *worksetMeasuredResponse) Flush() { _ = http.NewResponseController(w.ResponseWriter).Flush() }

// A second, explicitly requested diagnostic run stops the dispatcher before its
// single request, so summed driver read time cannot include unrelated polling.
func (f *worksetMeasurement) readRevisionBreakdown(t *testing.T, id, plan string) {
	t.Helper()
	if err := f.svc.DispatcherHandle().Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	measured := &worksetRevisionTiming{Service: f.svc}
	handler := httpapi.NewServer(httpapi.Dependencies{WorksetService: measured})
	f.sql.readNanos.Store(0)
	f.sql.readActive.Store(true)
	response := httptest.NewRecorder()
	started := time.Now()
	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/worksets/"+id+"/operations/conversion/revisions/"+plan, nil),
	)
	total := time.Since(started)
	f.sql.readActive.Store(false)
	if response.Code != http.StatusOK {
		t.Fatalf("diagnostic status=%d", response.Code)
	}
	query := time.Duration(f.sql.readNanos.Load())
	t.Logf(
		"revision_breakdown query_ms=%.3f view_ms=%.3f adapter_encode_ms=%.3f total_ms=%.3f bytes=%d",
		float64(query)/float64(time.Millisecond),
		float64(measured.elapsed-query)/float64(time.Millisecond),
		float64(total-measured.elapsed)/float64(time.Millisecond),
		float64(total)/float64(time.Millisecond),
		response.Body.Len(),
	)
}

type worksetRevisionTiming struct {
	workset.Service

	elapsed time.Duration
}

func (s *worksetRevisionTiming) GetRevision(ctx context.Context, id, kind, plan string) (*workset.RevisionView, error) {
	started := time.Now()
	view, err := s.Service.GetRevision(ctx, id, kind, plan)
	s.elapsed = time.Since(started)
	return view, err
}

// QueryContext, Next and Close measure time inside the SQL driver; row Scan,
// payload decoding, validation and conclusion assembly remain in view time.
// Pool waiting is excluded, matching the lifecycle's transaction measurements.
type worksetReadTimedRows struct {
	driver.Rows

	elapsed *atomic.Int64
}

func (r *worksetReadTimedRows) Next(values []driver.Value) error {
	started := time.Now()
	err := r.Rows.Next(values)
	r.elapsed.Add(int64(time.Since(started)))
	return err
}
func (r *worksetReadTimedRows) Close() error {
	started := time.Now()
	err := r.Rows.Close()
	r.elapsed.Add(int64(time.Since(started)))
	return err
}

// Replanning the first run's real generated MP3 inventory freezes 12 keep
// decisions per member, rather than testing the new field with empty arrays.
func (f *worksetMeasurement) keptSnapshot(t *testing.T, id, member string) {
	t.Helper()
	plan := f.generate(t, id, workset.OperationTypeConversion, 1)
	execution := f.execute(t, id, workset.OperationTypeConversion, plan, 1)
	f.read(
		t,
		"execution/conversion/kept-first-page",
		"/api/v1/worksets/"+id+"/operations/conversion/executions/"+execution+"?components_from=0&components_limit=200",
	)
	f.browser(t, "execution", id, plan, member, execution)
}

// measureConversionRound returns true when an explicitly shortened diagnostic
// or retained-snapshot run has completed instead of the full lifecycle.
func (f *worksetMeasurement) measureConversionRound(t *testing.T, id string, round int) bool {
	t.Helper()
	conversionPlan := f.generate(t, id, workset.OperationTypeConversion, round)
	f.read(t, "revision/conversion", "/api/v1/worksets/"+id+"/operations/conversion/revisions/"+conversionPlan)
	memberID := f.readLargestMember(t, id, conversionPlan)
	if os.Getenv("PREEN_MEASURE_READ_BREAKDOWN") == "1" {
		f.readRevisionBreakdown(t, id, conversionPlan)
		return true
	}
	if round == 0 && os.Getenv("PREEN_MEASURE_KEPT_SNAPSHOT") != "1" {
		f.browser(t, "revision", id, conversionPlan, memberID, "")
	}
	executionID := f.execute(t, id, workset.OperationTypeConversion, conversionPlan, round)
	f.read(
		t,
		"execution/conversion/first-page",
		"/api/v1/worksets/"+id+"/operations/conversion/executions/"+executionID+"?components_from=0&components_limit=200",
	)
	if os.Getenv("PREEN_MEASURE_KEPT_SNAPSHOT") == "1" {
		f.keptSnapshot(t, id, memberID)
		return true
	}
	f.browser(t, "execution", id, conversionPlan, memberID, executionID)
	return false
}
