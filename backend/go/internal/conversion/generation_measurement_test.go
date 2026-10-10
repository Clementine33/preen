package conversion_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/adapters/media"
	appconfig "github.com/onsei/organizer/backend/internal/adapters/settings"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/conversion"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/workset"
)

type generationMeasurement struct {
	Total, Walk, WalkMax, Staging, Merge, Backpressure time.Duration
	FolderWalks                                        []time.Duration
	Folders                                            int
	GainRatio                                          float64
}

type measuredStaging struct {
	*sqlite.ScanStaging

	measurement *generationMeasurement
}

func (m measuredStaging) WriteStagingBatch(id string, entries []inventory.StagingEntry) error {
	start := time.Now()
	err := m.ScanStaging.WriteStagingBatch(id, entries)
	m.measurement.Staging += time.Since(start)
	return err
}

func (m measuredStaging) MergeSelectedStaging(
	ctx context.Context,
	id, root string,
	scopes []string,
) (inventory.MergeOutcome, error) {
	start := time.Now()
	result, err := m.ScanStaging.MergeSelectedStaging(ctx, id, root, scopes)
	m.measurement.Merge += time.Since(start)
	return result, err
}

func (m measuredStaging) MergeStaging(id, library, root string) (inventory.MergeOutcome, error) {
	start := time.Now()
	result, err := m.ScanStaging.MergeStaging(id, library, root)
	m.measurement.Merge += time.Since(start)
	return result, err
}

// TestMeasureConversionGeneration runs five actual generation sessions against
// copies of one quiescent SQLite backup. It never writes the source database or
// media. Set CONVERSION_MEASURE_DB and CONVERSION_MEASURE_WORKSET to opt in.
func TestMeasureConversionGeneration(t *testing.T) {
	database := os.Getenv("CONVERSION_MEASURE_DB")
	worksetID := os.Getenv("CONVERSION_MEASURE_WORKSET")
	if database == "" || worksetID == "" {
		t.Skip("manual generation measurement requires a database backup and workset")
	}
	initial, readErr := os.ReadFile(database)
	if readErr != nil {
		t.Fatal(readErr)
	}
	rounds := 5
	if os.Getenv("CONVERSION_MEASURE_CANDIDATE") != "" {
		rounds = 10
	}
	measurements := make([]generationMeasurement, rounds)
	for round := range measurements {
		measureGenerationRound(t, worksetID, initial, round, &measurements[round])
		t.Logf("round %d: %+v", round+1, measurements[round])
	}
	if rounds == 5 {
		ratios := make([]float64, len(measurements))
		for i, m := range measurements {
			ratios[i] = m.GainRatio
		}
		slices.Sort(ratios)
		t.Logf("median estimated gain: %.2f%%; candidate gate: %t", ratios[2]*100, ratios[2] >= .2)
	} else {
		baseline, candidate := []time.Duration{}, []time.Duration{}
		improved := 0
		for i := 0; i < len(measurements); i += 2 {
			baseline = append(baseline, measurements[i].Total)
			candidate = append(candidate, measurements[i+1].Total)
			if measurements[i+1].Total < measurements[i].Total {
				improved++
			}
		}
		slices.Sort(baseline)
		slices.Sort(candidate)
		gain := 1 - float64(candidate[2])/float64(baseline[2])
		t.Logf(
			"paired median gain %.2f%%; improved %d/5; delivery gate: %t",
			gain*100,
			improved,
			gain >= .05 && improved >= 4,
		)
	}
	if output := os.Getenv("CONVERSION_MEASURE_OUTPUT"); output != "" {
		data, err := json.MarshalIndent(measurements, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func measureGenerationRound(
	t *testing.T,
	worksetID string,
	initial []byte,
	round int,
	m *generationMeasurement,
) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, openErr := sqlite.NewDevelopmentRepository(path)
	if openErr != nil {
		t.Fatal(openErr)
	}
	staging := measuredStaging{ScanStaging: sqlite.NewScanStaging(repo), measurement: m}
	walk := measuredWalk(m)
	pipeline := inventory.NewPipeline(staging, filesystem.WalkRootEntriesParallel, walk, nil)
	executionIO := conversion.ExecutionIO{}
	if os.Getenv("CONVERSION_MEASURE_CANDIDATE") != "" && round%2 == 1 {
		executionIO.Scan = func(ctx context.Context, roots []string, base string, check func()) error {
			_, err := pipeline.ScanFoldersCtx(
				ctx,
				roots,
				base,
				32,
				inventory.WithProgress(func(inventory.Progress) { check() }),
			)
			return err
		}
	}
	task := conversion.New(
		repo,
		appconfig.NewReader(dir),
		ffmpeg.New,
		media.ProbeBitrate,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		executionIO,
	)
	svc := workset.NewService(
		repo,
		1,
		1,
		[]workset.Task{task},
		func(ctx context.Context, folder, root string) error {
			_, err := pipeline.ScanFolderCtx(ctx, folder, root)
			return err
		},
		nil,
	)
	op, operationErr := svc.GetOperation(t.Context(), worksetID, "conversion")
	if operationErr != nil {
		t.Fatal(operationErr)
	}
	// This benchmark's input backup must have no reusable current revision.
	if op.CurrentRevision != nil {
		t.Fatal("measurement backup has a reusable revision")
	}
	svc.DispatcherHandle().Start()
	start := time.Now()
	result, startErr := svc.StartGeneration(
		t.Context(),
		worksetID,
		"conversion",
		workset.StartGenerationRequest{IfMatchVersion: op.Version, IdempotencyKey: "measurement"},
	)
	if startErr != nil {
		t.Fatal(startErr)
	}
	if !result.Created {
		t.Fatal("measurement reused a plan")
	}
	for {
		generation, err := svc.GetGeneration(t.Context(), worksetID, "conversion", result.Generation.GenerationID)
		if err != nil {
			t.Fatal(err)
		}
		if generation.Status == workset.GenStatusCompleted {
			break
		}
		if generation.Status == workset.GenStatusFailed {
			t.Fatalf("generation failed: %+v", generation)
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	m.Total = time.Since(start)
	if stopErr := svc.DispatcherHandle().Stop(context.Background()); stopErr != nil {
		t.Error(stopErr)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	setEstimatedGain(m)
}

func setEstimatedGain(m *generationMeasurement) {
	workers := min(32, m.Folders)
	if workers > 0 {
		gain := max(time.Duration(0), m.Walk-max(m.Walk/time.Duration(workers), m.WalkMax))
		m.GainRatio = float64(min(m.Total, gain)) / float64(m.Total) // Gdb = 0 without isolated evidence.
	}
}

func measuredWalk(
	m *generationMeasurement,
) func(context.Context, string, string, func(inventory.DirEntry) error) error {
	var walkMu sync.Mutex
	return func(ctx context.Context, folder, root string, emit func(inventory.DirEntry) error) error {
		var waiting time.Duration
		start := time.Now()
		err := filesystem.WalkFolderEntries(ctx, folder, root, func(entry inventory.DirEntry) error {
			callStart := time.Now()
			emitErr := emit(entry)
			waiting += time.Since(callStart)
			return emitErr
		})
		pure := time.Since(start) - waiting
		walkMu.Lock()
		m.Walk += pure
		m.WalkMax = max(m.WalkMax, pure)
		m.Backpressure += waiting
		m.FolderWalks = append(m.FolderWalks, pure)
		m.Folders++
		walkMu.Unlock()
		return err
	}
}
