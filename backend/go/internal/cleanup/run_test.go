package cleanup_test

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

type batchStore struct {
	*sqlite.Repository

	committed chan struct{}
	failAfter int
	batches   int
}

func (s *batchStore) CleanupBatch(
	ctx context.Context,
	id string,
	category cleanup.Category,
	cutoff time.Time,
	ids []string,
) error {
	if s.failAfter > 0 && s.batches >= s.failAfter {
		return errors.New("injected failure")
	}
	if err := s.Repository.CleanupBatch(ctx, id, category, cutoff, ids); err != nil {
		return err
	}
	s.batches++
	select {
	case s.committed <- struct{}{}:
	default:
	}
	return nil
}
func newCleanup(t *testing.T, delay time.Duration, failAfter int) (*cleanup.Service, *batchStore, *admission.Gate) {
	t.Helper()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	for i := range 3 {
		if err := repo.CreateScanSession(
			&inventory.ScanSession{
				SessionID: strconv.Itoa(i),
				RootPath:  "/music",
				Kind:      "full",
				Status:    "completed",
				StartedAt: time.Now().Add(-48 * time.Hour),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	store := &batchStore{Repository: repo, committed: make(chan struct{}, 3), failAfter: failAfter}
	gate := admission.NewGate(nil)
	svc := cleanup.NewService(
		context.Background(),
		store,
		gate,
		cleanup.Options{
			ScanRetention:       24 * time.Hour,
			GenerationRetention: 72 * time.Hour,
			BatchRows:           1,
			YieldDelay:          delay,
		},
	)
	t.Cleanup(svc.Shutdown)
	return svc, store, gate
}
func awaitCommit(t *testing.T, store *batchStore) {
	t.Helper()
	select {
	case <-store.committed:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not commit")
	}
}
func awaitTerminal(t *testing.T, svc *cleanup.Service) *cleanup.Run {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		run, err := svc.Read(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if run != nil && run.FinishedAt != nil {
			return run
		}
		select {
		case <-deadline:
			t.Fatal("cleanup did not finish")
		case <-time.After(time.Millisecond):
		}
	}
}
func TestCancelIsProcessOwnedAndRetainsCommittedResults(t *testing.T) {
	svc, store, gate := newCleanup(t, time.Hour, 0)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	run, err := svc.Start(requestCtx, []cleanup.Category{cleanup.ScanSessions})
	if err != nil {
		t.Fatal(err)
	}
	cancelRequest()
	awaitCommit(t, store)
	stopped, err := svc.Cancel(t.Context(), run.ID)
	if err != nil || stopped.Status != "stopping" {
		t.Fatalf("stop %+v %v", stopped, err)
	}
	result := awaitTerminal(t, svc)
	if result.Status != "stopped" || result.Counts[cleanup.ScanSessions].Deleted != 1 {
		t.Fatalf("result %+v", result)
	}
	svc.Shutdown()
	release, err := gate.BeginMaintenance()
	if err != nil {
		t.Fatal(err)
	}
	release()
	again, err := svc.Cancel(t.Context(), run.ID)
	if err != nil || again.Status != "stopped" {
		t.Fatalf("repeat %+v %v", again, err)
	}
}
func TestShutdownJoinsAndInterruptsCleanup(t *testing.T) {
	svc, store, gate := newCleanup(t, time.Hour, 0)
	if _, err := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions}); err != nil {
		t.Fatal(err)
	}
	awaitCommit(t, store)
	svc.Shutdown()
	run, err := svc.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "interrupted" || run.Counts[cleanup.ScanSessions].Deleted != 1 {
		t.Fatalf("shutdown %+v", run)
	}
	release, err := gate.BeginMaintenance()
	if err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestFailedBatchPreservesEarlierCommitAndReleasesGate(t *testing.T) {
	svc, _, gate := newCleanup(t, time.Millisecond, 1)
	if _, err := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions}); err != nil {
		t.Fatal(err)
	}
	run := awaitTerminal(t, svc)
	svc.Shutdown()
	if run.Status != "failed" || run.Counts[cleanup.ScanSessions].Deleted != 1 {
		t.Fatalf("failure %+v", run)
	}
	release, err := gate.BeginMaintenance()
	if err != nil {
		t.Fatal(err)
	}
	release()
}
func TestMultipleClientsOnlyStartOneRun(t *testing.T) {
	svc, store, _ := newCleanup(t, time.Hour, 0)
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		wg.Go(func() { _, err := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions}); results <- err })
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !admission.IsBusy(err) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d runs", accepted)
	}
	awaitCommit(t, store)
}
func TestNormalActivityInterruptsCleanupAtReentry(t *testing.T) {
	svc, store, gate := newCleanup(t, 100*time.Millisecond, 0)
	if _, err := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions}); err != nil {
		t.Fatal(err)
	}
	awaitCommit(t, store)
	var release func()
	deadline := time.After(time.Second)
	for {
		var err error
		release, err = gate.BeginScan()
		if err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal(err)
		case <-time.After(time.Millisecond):
		}
	}
	defer release()
	result := awaitTerminal(t, svc)
	if result.Status != "interrupted" || result.Counts[cleanup.ScanSessions].Deleted != 1 {
		t.Fatalf("reentry %+v", result)
	}
}

func TestStartupRecoveryInterruptsPersistedRunWithoutResuming(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	repo, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := cleanup.Run{
		ID:         "old",
		Status:     "stopping",
		Categories: []cleanup.Category{cleanup.ScanSessions},
		Counts:     map[cleanup.Category]cleanup.Count{},
	}
	saved.Counts[cleanup.ScanSessions] = cleanup.Count{Checked: 5, Deleted: 4, Skipped: 1}
	if createErr := repo.CreateCleanupRun(t.Context(), saved); createErr != nil {
		t.Fatal(createErr)
	}
	if closeErr := repo.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	reopened, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	svc := cleanup.NewService(
		context.Background(),
		reopened,
		admission.NewGate(nil),
		cleanup.Options{ScanRetention: time.Hour, GenerationRetention: time.Hour},
	)
	defer svc.Shutdown()
	if recoverErr := svc.Recover(t.Context()); recoverErr != nil {
		t.Fatal(recoverErr)
	}
	run, err := svc.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != "old" || run.Status != "interrupted" || run.FinishedAt == nil ||
		run.Counts[cleanup.ScanSessions] != (cleanup.Count{Checked: 5, Deleted: 4, Skipped: 1}) {
		t.Fatalf("recovered %+v", run)
	}
}
