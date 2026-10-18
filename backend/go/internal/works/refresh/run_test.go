package refresh_test

import (
	"context"
	"encoding/json"
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

type batchSource struct{ cancel context.CancelFunc }

func (s batchSource) FetchStatic(ctx context.Context, id works.WorkCode, _ string) (refresh.Snapshot, error) {
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
