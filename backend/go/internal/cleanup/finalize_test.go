package cleanup_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
)

type finalizingStore struct {
	*sqlite.Repository

	first   sync.Once
	entered chan struct{}
	proceed chan struct{}
	failed  chan error
}

func (s *finalizingStore) FinishCleanupRun(ctx context.Context, id, status, reason string) error {
	s.first.Do(func() { close(s.entered); <-s.proceed })
	err := s.Repository.FinishCleanupRun(ctx, id, status, reason)
	if err != nil {
		select {
		case s.failed <- err:
		default:
		}
	}
	return err
}

func TestCleanupFinalizationWaitsForCompetingWriter(t *testing.T) {
	for _, mode := range []string{"next run", "user stop", "shutdown join"} {
		t.Run(mode, func(t *testing.T) { testCleanupFinalizationWriter(t, mode) })
	}
}

func testCleanupFinalizationWriter(t *testing.T, mode string) {
	t.Helper()
	shutdown := mode == "shutdown join"
	svc, store, gate, writer := newFinalizingCleanup(t)
	started, err := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("finalization did not start")
	}
	if mode == "user stop" {
		if _, stopErr := svc.Cancel(t.Context(), started.ID); stopErr != nil {
			t.Fatal(stopErr)
		}
	}
	tx, err := writer.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	if _, writeErr := tx.ExecContext(t.Context(), "UPDATE cleanup_run SET summary=summary"); writeErr != nil {
		t.Fatal(writeErr)
	}
	blockedAt := time.Now()
	close(store.proceed)
	select {
	case <-store.failed:
	case <-time.After(8 * time.Second):
		t.Fatal("expected contention to cross finalization deadline")
	}
	if time.Since(blockedAt) < 3*time.Second {
		t.Fatal("writer did not cross the original finalization timeout")
	}
	var shutdownDone chan struct{}
	if shutdown {
		shutdownDone = make(chan struct{})
		go func() { svc.Shutdown(); close(shutdownDone) }()
	}
	// Give the failed attempt time to return while the real writer remains active.
	time.Sleep(100 * time.Millisecond)
	if shutdown {
		select {
		case <-shutdownDone:
			t.Fatal("shutdown abandoned pending finalization")
		default:
		}
	}
	pendingStatus := "running"
	if mode == "user stop" {
		pendingStatus = "stopping"
	}
	assertPendingFinalization(t, svc, gate, started.ID, pendingStatus)
	if commitErr := tx.Commit(); commitErr != nil {
		t.Fatal(commitErr)
	}
	finished := awaitTerminal(t, svc)
	expected := "succeeded"
	if shutdown {
		expected = "interrupted"
	}
	if mode == "user stop" {
		expected = "stopped"
	}
	if finished.ID != started.ID || finished.Status != expected {
		t.Fatalf("durable summary %+v", finished)
	}
	if shutdown {
		select {
		case <-shutdownDone:
		case <-time.After(3 * time.Second):
			t.Fatal("shutdown did not join finalization")
		}
		release, gateErr := gate.BeginMaintenance()
		if gateErr != nil {
			t.Fatal(gateErr)
		}
		release()
		return
	}
	assertNextCleanupStarts(t, svc, started.ID)
}

func newFinalizingCleanup(t *testing.T) (*cleanup.Service, *finalizingStore, *admission.Gate, *sql.Conn) {
	t.Helper()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "finalize.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	writer, err := repo.DB().Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	store := &finalizingStore{
		Repository: repo,
		entered:    make(chan struct{}),
		proceed:    make(chan struct{}),
		failed:     make(chan error, 1),
	}
	gate := admission.NewGate(nil)
	svc := cleanup.NewService(
		context.Background(),
		store,
		gate,
		cleanup.Options{ScanRetention: time.Hour, GenerationRetention: time.Hour},
	)
	t.Cleanup(svc.Shutdown)
	t.Cleanup(func() {
		select {
		case <-store.proceed:
		default:
			close(store.proceed)
		}
	})
	return svc, store, gate, writer
}

func assertNextCleanupStarts(t *testing.T, svc *cleanup.Service, previousID string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		next, startErr := svc.Start(t.Context(), []cleanup.Category{cleanup.ScanSessions})
		if startErr == nil {
			if next.ID == previousID {
				t.Fatal("next run reused identity")
			}
			break
		}
		if !admission.IsBusy(startErr) {
			t.Fatal(startErr)
		}
		select {
		case <-deadline:
			t.Fatal("next run stayed blocked after finalization")
		case <-time.After(time.Millisecond):
		}
	}
}

func assertPendingFinalization(t *testing.T, svc *cleanup.Service, gate *admission.Gate, id, pendingStatus string) {
	t.Helper()
	if release, busyErr := gate.BeginMaintenance(); !admission.IsBusy(busyErr) {
		if release != nil {
			release()
		}
		t.Fatalf("pending finalization lost its run marker: %v", busyErr)
	}
	pending, err := svc.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if pending.ID != id || pending.Status != pendingStatus {
		t.Fatalf("pending summary %+v", pending)
	}
}
