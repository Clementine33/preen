package refresh_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/works/refresh"

	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/workset"
)

type batchSource struct {
	cancel      context.CancelFunc
	beforeFetch func()
}

func (s batchSource) FetchStatic(ctx context.Context, id works.WorkCode, _ string) (refresh.Snapshot, error) {
	if s.beforeFetch != nil {
		s.beforeFetch()
	}
	if id == "RJ00000001" {
		return refresh.Snapshot{}, fmt.Errorf("work unavailable")
	}
	if s.cancel != nil {
		s.cancel()
		return refresh.Snapshot{}, ctx.Err()
	}
	title := "second"
	var snapshot refresh.Snapshot
	snapshot.Core = &works.ImportCore{Source: "dlsite", FetchedAt: "2026-10-01T00:00:00.000000000Z", Title: &title}
	snapshot.Editions = &works.ImportEditions{Source: "dlsite", FetchedAt: "2026-10-01T00:00:00.000000000Z"}
	return snapshot, nil
}
func (batchSource) FetchASMR(context.Context, works.WorkCode, string) (refresh.Snapshot, error) {
	return refresh.Snapshot{}, fmt.Errorf("backup unavailable")
}

func (batchSource) FetchDynamic(
	_ context.Context,
	ids []works.WorkCode,
	_ string,
) (map[works.WorkCode]works.ImportRequest, error) {
	out := map[works.WorkCode]works.ImportRequest{}
	for _, id := range ids {
		out[id] = works.ImportRequest{
			Statistics: &works.ImportStatistics{FetchedAt: "2026-10-01T00:00:00.000000000Z"},
			Relations: &works.ImportRelations{
				Source:    "dlsite",
				FetchedAt: "2026-10-01T00:00:00.000000000Z",
				Role:      "original",
			},
		}
	}
	return out, nil
}
func taskBatchCatalog(t *testing.T) (*sqlite.Repository, *works.Service) {
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
	for _, name := range []string{"RJ00000001", "RJ00000002"} {
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
	return repo, works.NewService(repo, nil, nil, nil, "")
}
func batchInput(progress func(int, workset.UnitResult) error) workset.BatchExecutionInput {
	in := workset.BatchExecutionInput{
		Options:  json.RawMessage(`{"mode":"refresh_all","locale":"ja-jp","download_cover":false}`),
		Progress: progress,
	}
	for i, id := range []string{"RJ00000001", "RJ00000002"} {
		raw, _ := json.Marshal(
			refresh.Outcome{Action: "refresh", Target: coverdelivery.Target{CopyID: id, WorkCode: id}},
		)
		in.Units = append(in.Units, workset.ExecutionUnit{Index: i, Operations: 1, Payload: raw})
	}
	return in
}
func TestBatchContinuesAfterOneWorkFails(t *testing.T) {
	repo, cat := taskBatchCatalog(t)
	source := batchSource{}
	executor := refresh.Executor{Metadata: refresh.NewService(cat, repo, source, source)}
	results := map[int]workset.UnitResult{}
	if err := executor.Run(
		t.Context(),
		batchInput(func(i int, r workset.UnitResult) error { results[i] = r; return nil }),
	); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ErrorCode != "METADATA_UNAVAILABLE" || results[1].ErrorCode != "" {
		t.Fatalf("partial results: %+v", results)
	}
	detail, err := cat.Detail("RJ00000002")
	if err != nil || detail.Title == nil || *detail.Title != "second" {
		t.Fatalf("second work not committed: %+v %v", detail, err)
	}
}
func TestBatchCancellationDoesNotImportNewStaticResults(t *testing.T) {
	repo, cat := taskBatchCatalog(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := batchSource{cancel: cancel}
	executor := refresh.Executor{Metadata: refresh.NewService(cat, repo, source, source)}
	if err := executor.Run(ctx, batchInput(func(int, workset.UnitResult) error { return nil })); err == nil {
		t.Fatal("cancellation ignored")
	}
	detail, err := cat.Detail("RJ00000002")
	if err != nil || detail.Title != nil {
		t.Fatalf("canceled source imported: %+v %v", detail, err)
	}
}

func withNonFetchUnits(in workset.BatchExecutionInput) workset.BatchExecutionInput {
	for _, outcome := range []refresh.Outcome{
		{Action: "skip", ReasonCode: "NO_WORK_BINDING"},
		{Action: "skip", ReasonCode: "ALREADY_COMPLETE"},
		{Action: "skip", ReasonCode: "SOURCE_MISSING"},
		{Action: "cover", Target: coverdelivery.Target{CopyID: "copy", WorkCode: "RJ00000002"}},
	} {
		raw, _ := json.Marshal(outcome)
		in.Units = append(in.Units, workset.ExecutionUnit{Index: len(in.Units), Payload: raw})
	}
	return in
}

func TestBatchReportsNonFetchUnitsOnceBeforeFetching(t *testing.T) {
	repo, cat := taskBatchCatalog(t)
	batchCalls, singleCalls := 0, 0
	in := withNonFetchUnits(batchInput(func(index int, _ workset.UnitResult) error {
		if index > 1 || batchCalls != 1 {
			t.Fatalf("unexpected single progress: index=%d, batches=%d", index, batchCalls)
		}
		singleCalls++
		return nil
	}))
	in.ProgressBatch = func(results map[int]workset.UnitResult) error {
		batchCalls++
		if len(results) != 4 {
			t.Fatalf("non-fetch results: %+v", results)
		}
		for _, unit := range in.Units[2:] {
			result, ok := results[unit.Index]
			if !ok || result.Stage != "skipped" || result.CompletedOperations == nil ||
				*result.CompletedOperations != 0 || string(result.TaskResult) != string(unit.Payload) {
				t.Fatalf("non-fetch result changed: %+v", result)
			}
		}
		return nil
	}
	source := batchSource{beforeFetch: func() {
		if batchCalls != 1 {
			t.Fatalf("fetch before batch progress: %d", batchCalls)
		}
	}}
	executor := refresh.Executor{Metadata: refresh.NewService(cat, repo, source, source)}
	if err := executor.Run(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if batchCalls != 1 || singleCalls != 2 {
		t.Fatalf("progress calls: batch=%d single=%d", batchCalls, singleCalls)
	}
}

func TestBatchStopsBeforeCoversAndFetchOnProgressFailureOrCancellation(t *testing.T) {
	writeErr := errors.New("batch write failed")
	for _, scenario := range []string{"write failure", "canceled before batch", "canceled after batch"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			in := withNonFetchUnits(batchInput(func(int, workset.UnitResult) error {
				t.Fatal("single progress after failure or cancellation")
				return nil
			}))
			in.Options = json.RawMessage(`{"mode":"refresh_all","locale":"ja-jp","download_cover":true}`)
			batchCalls := 0
			in.ProgressBatch = func(map[int]workset.UnitResult) error {
				batchCalls++
				if scenario == "write failure" {
					return writeErr
				}
				cancel()
				return nil
			}
			wantErr, wantCalls := writeErr, 1
			if scenario != "write failure" {
				wantErr = context.Canceled
			}
			if scenario == "canceled before batch" {
				cancel()
				wantCalls = 0
			}
			// Reaching either dependency would fail: both must follow batch persistence.
			err := (refresh.Executor{}).Run(ctx, in)
			if !errors.Is(err, wantErr) || batchCalls != wantCalls {
				t.Fatalf("Run: %v, batch calls=%d; want %v, %d", err, batchCalls, wantErr, wantCalls)
			}
		})
	}
}
