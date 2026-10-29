//nolint:testpackage // The opt-in harness uses the shared private driver instrumentation.
package sqlite

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestMeasureLibraryDeletion(t *testing.T) {
	if os.Getenv("PREEN_MEASURE_WORKSET") != "1" {
		t.Skip("set PREEN_MEASURE_WORKSET=1 to run synthetic lifecycle measurements")
	}
	libraryMembers := worksetMeasurementInt(t, "PREEN_MEASURE_MEMBERS", 5000)
	selected := worksetMeasurementInt(t, "PREEN_MEASURE_SELECTED", 20)
	audio := worksetMeasurementInt(t, "PREEN_MEASURE_AUDIO", 12)
	if selected > libraryMembers {
		t.Fatal("selected members must fit the library")
	}
	f := newWorksetMeasurement(t, libraryMembers, audio)
	f.names = f.names[:selected]
	id := f.create(t, "")
	conversionPlan := f.generate(t, id, workset.OperationTypeConversion, 0)
	f.execute(t, id, workset.OperationTypeConversion, conversionPlan, 0)
	metadataPlan := f.generate(t, id, refresh.Kind, 0)
	f.execute(t, id, refresh.Kind, metadataPlan, 0)
	f.deliver(t, id, 0)
	for _, table := range []string{"inventory_dirs", "inventory_files", "work_copies", "workset_members", "cover_deliveries"} {
		var n int
		if err := f.repo.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		t.Logf("delete_load table=%s rows=%d", table, n)
	}
	// Exclude construction and execution from the deletion-only SQL report.
	f.sql.mu.Lock()
	f.sql.paths = map[string]worksetSQLSample{}
	f.sql.mu.Unlock()
	defer f.sql.report(t)
	started := time.Now()
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/libraries/"+f.libraryID, nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete response=%d: %s", response.Code, response.Body.String())
	}
	t.Logf("library_delete library_members=%d selected=%d audio_per_member=%d elapsed_ms=%.3f",
		libraryMembers, selected, audio, float64(time.Since(started))/float64(time.Millisecond))
	var retired int
	if err := f.repo.DB().
		QueryRow(`SELECT COUNT(*) FROM worksets WHERE retired=1 AND library_id IS NULL`).
		Scan(&retired); err != nil || retired != 1 {
		t.Fatalf("record not retained for bounded cleanup: %d, %v", retired, err)
	}
	for _, table := range []string{"libraries", "inventory_dirs", "inventory_files", "work_copies"} {
		var n int
		if err := f.repo.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("synchronous deletion left %s=%d, %v", table, n, err)
		}
	}
	var deleted int64
	for {
		n, err := f.repo.DeleteRetiredPlanBatch(t.Context(), 1000)
		if err != nil {
			t.Fatal(err)
		}
		deleted += n
		if n == 0 {
			break
		}
	}
	t.Logf("deleted record cleanup rows=%d", deleted)
}
