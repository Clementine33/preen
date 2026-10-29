//nolint:testpackage // The opt-in harness instruments the repository's private connection pools.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	appconfig "github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func newWorksetMeasurement(t *testing.T, members, audio int) *worksetMeasurement {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "config.json"),
		[]byte(`{"prune":{"no_effect_tags":["SEなし"]}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	repo, stats := worksetMeasurementRepository(t, dir)
	root := filepath.Join(dir, "media")
	libraryID, names := worksetMeasurementLibrary(t, repo, root, members, audio)
	pipeline := inventory.NewPipeline(
		NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	if _, err := pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(libraryID)); err != nil {
		t.Fatal(err)
	}
	gate := admission.NewGate(repo.HasActiveSession)
	source := worksetMeasurementSource{version: &atomic.Uint32{}}
	metadata := refresh.NewService(works.NewService(repo, nil, nil, nil, ""), repo, source, source)
	covers, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: repo, Downloader: source, Source: source, Files: filesystem.CoverFiles{}, Acquire: gate.BeginManual,
		SyncInventory: func(_ context.Context, target coverdelivery.Target, p coverdelivery.Publication) error {
			var changed []inventory.InventoryFile
			if p.File != nil {
				changed = append(changed, *p.File)
			}
			return repo.SyncObservedInventory(target.LibraryRoot, p.Removed, changed, nil)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.PlanRetirementGuard = func(write func() error) error { return covers.WithPublication(t.Context(), write) }
	tasks := worksetMeasurementTasks(repo, dir, pipeline)
	executor := refresh.Executor{Metadata: metadata, Covers: covers}
	svc := workset.NewService(
		repo,
		1,
		4,
		tasks,
		nil,
		gate.Enqueue,
		map[string]workset.BatchExecutor{refresh.Kind: executor.Run},
	)
	svc.SetExclusiveEnqueueGuard(gate.EnqueueExclusive)
	settings, err := svc.GetTaskSettings(t.Context(), refresh.Kind)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SaveTaskSettings(
		t.Context(),
		refresh.Kind,
		[]byte(`{"mode":"refresh_all","locale":"ja-jp","download_cover":true}`),
		settings.ETag,
	); err != nil {
		t.Fatal(err)
	}
	svc.DispatcherHandle().Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if stopErr := svc.DispatcherHandle().Stop(ctx); stopErr != nil {
			t.Error(stopErr)
		}
	})
	// Discard setup samples; all following samples belong to the lifecycle.
	stats.mu.Lock()
	stats.paths = map[string]worksetSQLSample{}
	stats.mu.Unlock()
	return &worksetMeasurement{
		repo:      repo,
		svc:       svc,
		covers:    covers,
		sql:       stats,
		root:      root,
		libraryID: libraryID,
		names:     names,
		tasks:     tasks,
		source:    source,
		handler: httpapi.NewServer(
			httpapi.Dependencies{
				WorksetService: svc,
				Covers:         covers,
				Library:        library.NewService(repo, gate, repo, filesystem.ResolveMember),
			},
		),
	}
}

func worksetMeasurementRepository(t *testing.T, dir string) (*Repository, *worksetSQLMeasurement) {
	t.Helper()
	repo, err := NewDevelopmentRepository(filepath.Join(dir, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	stats := &worksetSQLMeasurement{paths: map[string]worksetSQLSample{}}
	measured := worksetMeasuredDriver{Driver: repo.db.Driver(), measurement: stats}
	original := repo.db
	repo.db = sql.OpenDB(repositoryConnector{driver: measured, dsn: repo.dbPath + connectionPragmas})
	if err = original.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := repo.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	return repo, stats
}

func worksetMeasurementLibrary(t *testing.T, repo *Repository, root string, members, audio int) (string, []string) {
	t.Helper()
	names := make([]string, members)
	for i := range names {
		names[i] = fmt.Sprintf("RJ%08d album", i+1)
		folder := filepath.Join(root, names[i])
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		for j := range audio {
			if err := os.WriteFile(
				filepath.Join(folder, fmt.Sprintf("%02d.flac", j+1)),
				[]byte("synthetic audio"),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
		}
	}
	lib, err := repo.CreateLibrary("measurement", root)
	if err != nil {
		t.Fatal(err)
	}
	return lib.ID, names
}

func worksetMeasurementTasks(repo *Repository, dir string, pipeline *inventory.Pipeline) []workset.Task {
	return []workset.Task{
		conversion.New(
			repo,
			appconfig.NewReader(dir),
			func() execute.Encoder { return worksetMeasurementEncoder{} },
			func(context.Context, string) (int64, error) { return 320000, nil },
			filesystem.Recover,
			filesystem.IsResolvedWithinRoot,
			conversion.ExecutionIO{
				Walk: func(ctx context.Context, roots []string, base string, emit func(string, inventory.DirEntry) error) error {
					return filesystem.WalkSelectedEntriesParallel(ctx, roots, base, inventory.DefaultScanWorkers, emit)
				},
				Moves: filesystem.ConversionMoves{},
				Files: filesystem.ConversionHandles{},
				Scan: func(ctx context.Context, folders []string, root string, check func()) error {
					_, scanErr := pipeline.ScanFoldersCtx(ctx, folders, root, inventory.DefaultScanWorkers,
						inventory.WithProgress(func(inventory.Progress) { check() }))
					return scanErr
				},
			},
		),
		refresh.NewTask(repo, filesystem.CheckWorkDirectory),
	}
}
