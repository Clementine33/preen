package sqlite_test

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// TestMeasureComponentResultByRequestSize saves the same number of component
// results into sessions that differ only in the size of their frozen request,
// so any growth in save time is attributable to the session row's size.
// Opt-in: PREEN_MEASURE_EXEC_PAYLOAD=1.
func TestMeasureComponentResultByRequestSize(t *testing.T) {
	if os.Getenv("PREEN_MEASURE_EXEC_PAYLOAD") != "1" {
		t.Skip("set PREEN_MEASURE_EXEC_PAYLOAD=1 to measure")
	}
	const saves = 2000
	for _, size := range []int{0, 64 << 10, 512 << 10, 2 << 20, 8 << 20} {
		repo := newExecutionRepo(t)
		seedExecutionWorkset(t, repo, "ws-1", t.TempDir())
		e := newExecution("exec-1", "ws-1", "plan-1", "key")
		e.RequestJSON = `{"pad":"` + strings.Repeat("x", size) + `"}`
		insertExecutionRow(t, repo, e)
		var slowest time.Duration
		started := time.Now()
		for i := range saves {
			one := time.Now()
			if err := repo.SaveExecutionComponentResult("exec-1", workset.ExecutionComponentResult{
				ComponentIndex:      i,
				Status:              "succeeded",
				CompletedOperations: 1,
				ResultJSON:          `{}`,
			}, workset.ExecutionPosition{CurrentComponentIndex: i + 1}); err != nil {
				t.Fatal(err)
			}
			slowest = max(slowest, time.Since(one))
		}
		total := time.Since(started)
		t.Logf("request_bytes=%s saves=%d total_ms=%.3f avg_ms=%.3f max_ms=%.3f",
			strconv.Itoa(len(e.RequestJSON)), saves,
			float64(total)/float64(time.Millisecond),
			float64(total)/float64(time.Millisecond)/saves,
			float64(slowest)/float64(time.Millisecond))
	}
}
