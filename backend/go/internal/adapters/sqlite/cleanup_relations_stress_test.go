package sqlite_test

import (
	"context"
	"database/sql/driver"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	driverSQLite "modernc.org/sqlite"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
)

// Run with PREEN_CLEANUP_STRESS=1 go test ./internal/adapters/sqlite
// -run TestCleanupReportGroupPerformance -count=1 -v. Uses only temporary databases.
//
//nolint:funlen,gocognit // One measured path keeps timing, contention and cancellation instrumentation together.
func TestCleanupReportGroupPerformance(t *testing.T) {
	if os.Getenv("PREEN_CLEANUP_STRESS") != "1" {
		t.Skip("opt-in real SQLite group/lock/cancel/memory measurement")
	}
	t.Logf(
		"runtime=%s platform=%s/%s CPUs=%d GOMAXPROCS=%d",
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.NumCPU(),
		runtime.GOMAXPROCS(0),
	)
	for _, size := range []int{10000, 50000} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			locked := make(chan time.Time, 1)
			deleting := make(chan struct{}, 1)
			lockFunction := reportMetricFunction(t, func() { locked <- time.Now() })
			deleteFunction := reportMetricFunction(t, func() { deleting <- struct{}{} })
			repo := deletionRepository(t)
			seedReportRing(t, repo, size)
			ids, err := repo.CleanupCandidates(t.Context(), cleanup.RelationReports, time.Now(), nil)
			if err != nil {
				t.Fatal(err)
			}
			groups, err := repo.CleanupGroups(t.Context(), cleanup.RelationReports, ids)
			if err != nil || len(groups) != 1 || len(groups[0]) != size {
				t.Fatalf("groups=%d err=%v", len(groups), err)
			}
			if createErr := repo.CreateCleanupRun(
				t.Context(),
				cleanup.Run{ID: "stress", Status: "running", Counts: make(map[cleanup.Category]cleanup.Count)},
			); createErr != nil {
				t.Fatal(createErr)
			}
			deletionSQL(t, repo, `ALTER TABLE works RENAME TO metric_works`)
			deletionSQL(t, repo, `CREATE VIEW works AS SELECT `+lockFunction+`(id) AS id FROM metric_works`)
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			sampleCtx, cancelSample := context.WithCancel(t.Context())
			defer cancelSample()
			sampled := make(chan uint64, 1)
			go sampleCleanupHeap(sampleCtx, sampled)
			committed := make(chan time.Time, 1)
			repo.OnCatalogChanged = func() { committed <- time.Now() }
			done := make(chan error, 1)
			started := time.Now()
			go func() { done <- repo.CleanupBatch(t.Context(), "stress", cleanup.RelationReports, time.Now(), ids) }()
			var lockAt time.Time
			select {
			case lockAt = <-locked:
			case batchErr := <-done:
				t.Fatalf("batch ended before lock instrumentation: %v", batchErr)
			case <-time.After(10 * time.Second):
				t.Fatal("batch did not enter write transaction")
			}
			contender := make(chan time.Time, 1)
			go func() {
				tx, beginErr := repo.DB().BeginTx(t.Context(), nil)
				if beginErr != nil {
					done <- beginErr
					return
				}
				contender <- time.Now()
				_ = tx.Rollback()
			}()
			if batchErr := <-done; batchErr != nil {
				t.Fatal(batchErr)
			}
			elapsed := time.Since(started)
			commitAt := <-committed
			repo.OnCatalogChanged = nil
			var acquired time.Time
			select {
			case acquired = <-contender:
			case <-time.After(10 * time.Second):
				t.Fatal("writer remained blocked")
			}
			cancelSample()
			peak := <-sampled
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			t.Logf(
				"reports=%d groups=1 batch_elapsed=%s write_lock_first_read_to_commit=%s competing_writer_wait=%s allocated_bytes=%d sampled_peak_heap_delta_bytes=%d baseline_heap_bytes=%d sampled_peak_heap_bytes=%d",
				size,
				elapsed,
				commitAt.Sub(lockAt),
				acquired.Sub(lockAt),
				after.TotalAlloc-before.TotalAlloc,
				max(peak, before.HeapAlloc)-before.HeapAlloc,
				before.HeapAlloc,
				peak,
			)
			// Measure the real Service.Cancel path, including its persisted stopping
			// write waiting behind the indivisible report transaction.
			deletionSQL(t, repo, `DROP VIEW works; ALTER TABLE metric_works RENAME TO works`)
			seedReportRing(t, repo, size)
			deletionSQL(
				t,
				repo,
				`CREATE TRIGGER report_metric BEFORE DELETE ON relation_reports BEGIN SELECT `+deleteFunction+`(OLD.subject_rj); END`,
			)
			svc := cleanup.NewService(t.Context(), repo, admission.NewGate(nil), cleanup.Options{YieldDelay: time.Hour})
			t.Cleanup(svc.Shutdown)
			run, err := svc.Start(t.Context(), []cleanup.Category{cleanup.RelationReports})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-deleting:
			case <-time.After(10 * time.Second):
				t.Fatal("worker did not enter report deletion")
			}
			cancelAt := time.Now()
			if _, cancelErr := svc.Cancel(t.Context(), run.ID); cancelErr != nil {
				t.Fatal(cancelErr)
			}
			deadline := time.Now().Add(10 * time.Second)
			final, readErr := repo.ReadCleanupRun(context.Background())
			for readErr == nil && final.FinishedAt == nil && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
				final, readErr = repo.ReadCleanupRun(context.Background())
			}
			cancelElapsed := time.Since(cancelAt)
			if readErr != nil || final.Status != "stopped" {
				t.Fatalf("final=%+v err=%v", final, readErr)
			}
			svc.Shutdown()
			t.Logf(
				"reports=%d cancellation_latency=%s committed_report_count=%d final_status=%s",
				size,
				cancelElapsed,
				final.Counts[cleanup.RelationReports].Deleted,
				final.Status,
			)
		})
	}
}

func seedReportRing(t *testing.T, repo *sqlite.Repository, size int) {
	t.Helper()
	deletionSQL(t, repo, `WITH RECURSIVE ids(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM ids WHERE i<?)
 INSERT INTO relation_reports SELECT printf('RJ%08d',100000+i),'original',NULL,NULL,NULL,'dlsite','' FROM ids`, size)
	deletionSQL(t, repo, `WITH RECURSIVE ids(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM ids WHERE i<?)
 INSERT INTO relation_declared_children SELECT printf('RJ%08d',100000+i),printf('RJ%08d',100000+(i%?)+1) FROM ids`, size, size)
}

func reportMetricFunction(t *testing.T, entered func()) string {
	t.Helper()
	function := "cleanup_metric_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var once sync.Once
	if err := driverSQLite.RegisterScalarFunction(
		function,
		1,
		func(_ *driverSQLite.FunctionContext, args []driver.Value) (driver.Value, error) {
			once.Do(entered)
			return args[0], nil
		},
	); err != nil {
		t.Fatal(err)
	}
	return function
}

func sampleCleanupHeap(ctx context.Context, sampled chan<- uint64) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	var peak uint64
	for {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		peak = max(peak, stats.HeapAlloc)
		select {
		case <-ctx.Done():
			sampled <- peak
			return
		case <-ticker.C:
		}
	}
}
