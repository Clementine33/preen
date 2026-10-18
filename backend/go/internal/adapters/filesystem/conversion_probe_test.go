package filesystem_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
)

func TestConversionProbeCleansScratchAndDoesNotCreateRecovery(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	moves := filesystem.ConversionMoves{}
	if err := moves.Probe(
		t.Context(),
		[]execute.MoveRequirement{
			{Source: filepath.Join(work, "output.mp3"), Target: filepath.Join(work, "output.mp3")},
			{WorkRoot: work, Source: filepath.Join(work, "old.wav"), Recovery: true},
		},
	); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "work" {
		t.Fatalf("probe created recovery directories: %v", entries)
	}
	entries, err = os.ReadDir(work)
	if err != nil || len(entries) != 0 {
		t.Fatalf("scratch left: %v, %v", entries, err)
	}
}

func TestConversionProbeDeduplicatesMountsAndUsesRecoveryPair(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	requirements := []execute.MoveRequirement{
		{Source: filepath.Join(work, "a.mp3"), Target: filepath.Join(work, "a.mp3")},
		{Source: filepath.Join(work, "b.mp3"), Target: filepath.Join(work, "b.mp3")},
		{WorkRoot: work, Source: filepath.Join(work, "old.wav"), Recovery: true},
	}
	calls := 0
	err := filesystem.ProbeConversionMovesForTest(
		t.Context(),
		requirements,
		func(string) (string, error) { return "1", nil },
		func(_ context.Context, src, dst string) error {
			calls++
			if src != work || dst != root {
				t.Fatalf("probe pair = %s -> %s", src, dst)
			}
			return nil
		},
	)
	if err != nil || calls != 1 {
		t.Fatalf("probe calls %d: %v", calls, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := filesystem.ProbeConversionMovesForTest(
		ctx,
		requirements,
		func(string) (string, error) { return "1", nil },
		func(context.Context, string, string) error { t.Fatal("canceled probe ran"); return nil },
	); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatal(err)
	}
}

func TestConversionProbeCanceledAfterRefusalCleansOwnedScratch(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	moves := filesystem.ConversionMoves{}
	err := filesystem.ProbeConversionDirectoryForTest(ctx, func(src, dst string) (execute.MoveResult, error) {
		result, err := moves.Move(src, dst)
		cancel()
		return result, err
	}, root, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("probe = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("scratch = %v, %v", entries, err)
	}
}

func TestConversionRecoveryNeverRetriesPossibleLanding(t *testing.T) {
	for _, failure := range []error{os.ErrExist, errors.New("file is being used by another process")} {
		root := filepath.Join(t.TempDir(), "work")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(root, "old.mp3")
		if err := os.WriteFile(source, []byte("old audio"), 0o600); err != nil {
			t.Fatal(err)
		}
		calls := 0
		destination, result, err := filesystem.ConversionRecoverForTest(
			t.Context(),
			root,
			source,
			func(src, dst string) (execute.MoveResult, error) {
				calls++
				if linkErr := os.Link(src, dst); linkErr != nil {
					t.Fatal(linkErr)
				}
				return execute.MoveResult{Unverified: true}, failure
			},
		)
		if err == nil || calls != 1 || !result.Unverified || destination == "" {
			t.Fatalf("recovery retried unknown landing: %s %+v %v, calls %d", destination, result, err, calls)
		}
		if _, statErr := os.Stat(destination); statErr != nil {
			t.Fatal("landed copy lost", statErr)
		}
	}
}

func TestConversionProbePermissionAndIOStayPrecheckFailures(t *testing.T) {
	for _, failure := range []error{os.ErrPermission, errors.New("IO failure")} {
		root := t.TempDir()
		err := filesystem.ProbeConversionDirectoryForTest(
			t.Context(),
			func(string, string) (execute.MoveResult, error) { return execute.MoveResult{}, failure },
			root,
			root,
		)
		if !errors.Is(err, failure) || errors.Is(err, execute.ErrMoveUnsupported) {
			t.Fatalf("probe failure misclassified: %v", err)
		}
	}
}
