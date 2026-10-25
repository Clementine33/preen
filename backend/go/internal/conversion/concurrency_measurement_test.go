package conversion_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/adapters/ffmpeg"
	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/media"
	appconfig "github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/dirrename"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

type stageMeasurement struct {
	Work  time.Duration // Sum of call durations; overlapping stages must not be added to wall time.
	Calls int
	Peak  int
}

type concurrencyMeasurement struct {
	Stages      map[string]*stageMeasurement
	Queue       []time.Duration
	Latency     []time.Duration
	Digests     []string
	Total       time.Duration
	ProbeErrors int
	mu          sync.Mutex
	active      map[string]int
}

func (m *concurrencyMeasurement) start(stage string) func() {
	start := time.Now()
	m.mu.Lock()
	if m.Stages[stage] == nil {
		m.Stages[stage] = &stageMeasurement{}
	}
	m.active[stage]++
	m.Stages[stage].Peak = max(m.Stages[stage].Peak, m.active[stage])
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.active[stage]--
		m.Stages[stage].Calls++
		m.Stages[stage].Work += time.Since(start)
	}
}

type concurrencyStore struct {
	*sqlite.Repository

	m *concurrencyMeasurement
}

func (s concurrencyStore) ObservedAudioEntries(root string) ([]inventory.ObservedAudio, error) {
	defer s.m.start("inventory_read")()
	return s.Repository.ObservedAudioEntries(root)
}

func (s concurrencyStore) UpdateEntryBitrates(updates []inventory.BitrateUpdate, batch bool) error {
	defer s.m.start("bitrate_write")()
	return s.Repository.UpdateEntryBitrates(updates, batch)
}

func (s concurrencyStore) PersistOperationRevision(
	generation, id, kind string, now time.Time, revision workset.OperationRevisionPersist,
) error {
	defer s.m.start("persist")()
	return s.Repository.PersistOperationRevision(generation, id, kind, now, revision)
}

type concurrencyStaging struct {
	*sqlite.ScanStaging

	m *concurrencyMeasurement
}

func (s concurrencyStaging) WriteStagingBatch(id string, entries []inventory.StagingEntry) error {
	defer s.m.start("staging")()
	return s.ScanStaging.WriteStagingBatch(id, entries)
}

func (s concurrencyStaging) MergeSelectedStaging(
	ctx context.Context, id, root string, scopes []string,
) (inventory.MergeOutcome, error) {
	defer s.m.start("merge")()
	return s.ScanStaging.MergeSelectedStaging(ctx, id, root, scopes)
}

type concurrencyTask struct {
	*conversion.Task

	m    *concurrencyMeasurement
	kind string
}

func (t concurrencyTask) Kind() string { return t.kind }

func (t concurrencyTask) PlanSession(ctx context.Context, in workset.PlanSessionInput) (*workset.PlanSnapshot, error) {
	defer t.m.start("plan")()
	snapshot, err := t.Task.PlanSession(ctx, in)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	t.m.mu.Lock()
	t.m.Digests = append(t.m.Digests, fmt.Sprintf("%x", sha256.Sum256(data)))
	t.m.mu.Unlock()
	return snapshot, nil
}

// TestMeasureGenerationConcurrency opts in with a quiescent backup and a comma-
// separated workset list. Only isolated DB copies are written; media is read-only.
// Internal-width candidates can use Go source overlays without adding product settings.
func TestMeasureGenerationConcurrency(t *testing.T) {
	database := os.Getenv("CONCURRENCY_MEASURE_DB")
	ids := strings.Split(os.Getenv("CONCURRENCY_MEASURE_WORKSETS"), ",")
	if database == "" || ids[0] == "" {
		t.Skip("manual concurrency measurement requires a database backup and worksets")
	}
	if _, err := os.Stat(database + "-wal"); !os.IsNotExist(err) {
		t.Fatal("use a quiescent standalone backup without a WAL sidecar")
	}
	initial, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]*concurrencyMeasurement, measurementInt(t, "CONCURRENCY_MEASURE_ROUNDS", 5))
	for round := range results {
		results[round] = measureConcurrentGeneration(t, initial, ids)
		if round > 0 && !slices.Equal(results[round].Digests, results[0].Digests) {
			t.Fatal("plan results changed between rounds; measurement is not comparable")
		}
		data, marshalErr := json.Marshal(results[round])
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		t.Logf("round %d: %s", round+1, data)
	}
	if output := os.Getenv("CONCURRENCY_MEASURE_OUTPUT"); output != "" {
		data, marshalErr := json.MarshalIndent(results, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := os.WriteFile(output, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func measurementInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 || number > 32 {
		t.Fatalf("%s must be an integer in 1..32", key)
	}
	return number
}

func measureConcurrentGeneration(t *testing.T, initial []byte, ids []string) *concurrencyMeasurement {
	t.Helper()
	dir := t.TempDir()
	database := filepath.Join(dir, "cache.db")
	if err := os.WriteFile(database, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, err := sqlite.NewDevelopmentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, id := range ids {
		// Force a real generation instead of revision reuse, on the copied DB only.
		if _, err := repo.DB().Exec(
			"UPDATE workset_operations SET current_revision_id = NULL WHERE workset_id = ?", id,
		); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("CONCURRENCY_MEASURE_MISSING") == "1" {
		inventoryfixture.ClearBitrates(t, repo)
	}
	m := &concurrencyMeasurement{
		Stages: map[string]*stageMeasurement{}, active: map[string]int{},
	}
	svc := measuredGenerationService(t, dir, repo, m)
	targets := measurementTargets(t, repo, ids)
	measureGenerationRequests(t, svc, targets, m)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := svc.DispatcherHandle().Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(m.Digests)
	return m
}

func measuredGenerationService(
	t *testing.T, configDir string, repo *sqlite.Repository, m *concurrencyMeasurement,
) workset.Service {
	t.Helper()
	walk := func(ctx context.Context, folder, root string, emit func(inventory.DirEntry) error) error {
		defer m.start("walk")()
		return filesystem.WalkFolderEntries(ctx, folder, root, emit)
	}
	pipeline := inventory.NewPipeline(
		concurrencyStaging{
			ScanStaging: sqlite.NewScanStaging(repo),
			m:           m,
		},
		filesystem.WalkRootEntriesParallel,
		walk,
		nil,
	)
	io := conversion.ExecutionIO{Scan: func(ctx context.Context, folders []string, root string, check func()) error {
		defer m.start("refresh")()
		_, err := pipeline.ScanFoldersCtx(ctx, folders, root, measurementInt(t, "CONCURRENCY_MEASURE_SCAN", 32),
			inventory.WithProgress(func(inventory.Progress) { check() }))
		return err
	}}
	probe := func(ctx context.Context, path string) (int64, error) {
		defer m.start("probe")()
		bitrate, err := media.ProbeBitrate(ctx, path)
		if err != nil || bitrate <= 0 {
			m.mu.Lock()
			m.ProbeErrors++
			m.mu.Unlock()
		}
		return bitrate, err
	}
	store := concurrencyStore{Repository: repo, m: m}
	task := conversion.New(
		store, appconfig.NewReader(configDir),
		func() execute.Encoder { return ffmpeg.New(ffmpeg.ToolsConfig{}) },
		probe, filesystem.Recover, filesystem.IsResolvedWithinRoot, io,
	)
	var tasks []workset.Task
	for i := range measurementInt(t, "CONCURRENCY_MEASURE_COPIES", 1) {
		tasks = append(tasks, concurrencyTask{Task: task, m: m, kind: measurementKind(i)})
	}
	if os.Getenv("CONCURRENCY_MEASURE_LIGHT") == "1" {
		tasks = append(
			tasks,
			dirrename.New(repo, filesystem.ReadRenameDirectory),
			refresh.NewTask(repo, filesystem.CheckWorkDirectory),
		)
	}
	svc := workset.NewService(store, measurementInt(t, "CONCURRENCY_MEASURE_SESSIONS", 1), 1,
		tasks, nil, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.DispatcherHandle().Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	return svc
}

type measurementTarget struct{ id, kind string }

func measurementKind(copyIndex int) string {
	if copyIndex == 0 {
		return "conversion"
	}
	return fmt.Sprintf("measure_conversion_%d", copyIndex)
}

// shortcut: duplicate one batch for shared-disk stress; use distinct libraries before tuning independent devices.
func measurementTargets(t *testing.T, repo *sqlite.Repository, ids []string) []measurementTarget {
	t.Helper()
	var targets []measurementTarget
	for i := range measurementInt(t, "CONCURRENCY_MEASURE_COPIES", 1) {
		kind := measurementKind(i)
		for _, id := range ids {
			if i > 0 {
				if _, err := repo.DB().Exec(`INSERT INTO workset_operations(workset_id, operation_type, version)
					SELECT workset_id, ?, version FROM workset_operations WHERE workset_id = ? AND operation_type = 'conversion'`, kind, id); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.DB().
					Exec(`INSERT INTO workset_operation_drafts(workset_id, operation_type, schema_version, draft_json, draft_hash)
					SELECT workset_id, ?, schema_version, draft_json, draft_hash FROM workset_operation_drafts
					WHERE workset_id = ? AND operation_type = 'conversion'`, kind, id); err != nil {
					t.Fatal(err)
				}
			}
			targets = append(targets, measurementTarget{id: id, kind: kind})
		}
	}
	if os.Getenv("CONCURRENCY_MEASURE_LIGHT") == "1" {
		for _, id := range ids {
			targets = append(
				targets,
				measurementTarget{id: id, kind: "rename"},
				measurementTarget{id: id, kind: refresh.Kind},
			)
		}
	}
	return targets
}

func measureGenerationRequests(
	t *testing.T,
	svc workset.Service,
	targets []measurementTarget,
	m *concurrencyMeasurement,
) {
	t.Helper()
	requests := make([]*workset.StartGenerationResult, len(targets))
	m.Queue = make([]time.Duration, len(targets))
	m.Latency = make([]time.Duration, len(targets))
	start := time.Now()
	// Enqueue the whole workload before starting workers, so scan admission does
	// not make request order determine which sessions enter the measurement.
	for i, target := range targets {
		op, err := svc.GetOperation(t.Context(), target.id, target.kind)
		if err != nil {
			t.Fatal(err)
		}
		requests[i], err = svc.StartGeneration(t.Context(), target.id, target.kind, workset.StartGenerationRequest{
			IfMatchVersion: op.Version, IdempotencyKey: "concurrency-measurement",
		})
		if err != nil || !requests[i].Created {
			t.Fatalf("enqueue request %d: %v", i, err)
		}
	}
	svc.DispatcherHandle().Start()
	remaining := len(targets)
	for remaining > 0 {
		for i, request := range requests {
			if request == nil {
				continue
			}
			generation, err := svc.GetGeneration(
				t.Context(),
				targets[i].id,
				targets[i].kind,
				request.Generation.GenerationID,
			)
			if err != nil {
				t.Fatal(err)
			}
			switch generation.Status {
			case workset.GenStatusCompleted:
				m.Queue[i] = generation.StartedAt.Sub(generation.CreatedAt)
				m.Latency[i] = time.Since(start)
				requests[i] = nil
				remaining--
			case workset.GenStatusFailed, workset.GenStatusCanceled, workset.GenStatusInterrupted:
				t.Fatalf("request %d ended as %s: %s", i, generation.Status, generation.ErrorCode)
			}
		}
		if remaining > 0 {
			select {
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			case <-time.After(5 * time.Millisecond):
			}
		}
	}
	m.Total = time.Since(start)
}
