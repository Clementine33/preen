package execute_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

func TestComponentChecksAllStagedIdentitiesBeforeFirstPublish(t *testing.T) {
	root, wav := newAudioRoot(t)
	spec := mp3Spec()
	targets := []string{filepath.Join(root, "a.mp3"), filepath.Join(root, "b.mp3"), filepath.Join(root, "c.mp3")}
	prepared, err := execute.PrepareComponent(
		t.Context(),
		runRequest(root, componentFixture(
			[]reconcile.FileTuple{freezeFile(t, wav)},
			encodeOp(
				"cmp-1",
				wav,
				targets[0],
			),
			encodeOp("cmp-1", wav, targets[1]),
			encodeOp("cmp-1", wav, targets[2]),
		), reconcile.DesiredProfile{Encoded: &spec}, execute.DeleteModeHard),
	)
	if err != nil {
		t.Fatal(err)
	}
	for i := range prepared.Encodes() {
		if encodeErr := prepared.EncodeOne(t.Context(), i); encodeErr != nil {
			t.Fatal(encodeErr)
		}
	}
	for _, path := range tempFiles(t, root) {
		if strings.Contains(filepath.Base(path), "c.mp3") {
			if truncateErr := os.Truncate(path, 0); truncateErr != nil {
				t.Fatal(truncateErr)
			}
		}
	}
	result, err := prepared.Commit(t.Context())
	requireStage(t, err, execute.ComponentStageValidate)
	if len(result.Committed) != 0 {
		t.Fatalf("published before component validation: %+v", result)
	}
	for _, target := range targets {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("target published: %s", target)
		}
	}
}

func TestComponentRechecksEachPublishAndRetainsEarlierOutput(t *testing.T) {
	root, wav := newAudioRoot(t)
	spec := mp3Spec()
	first, second := filepath.Join(root, "a.mp3"), filepath.Join(root, "b.mp3")
	moves := filesystem.ConversionMoves{}
	kit := execute.ComponentRunTestKit{Move: func(source, target string) (execute.MoveResult, error) {
		result, err := moves.Move(source, target)
		if target == first && err == nil {
			for _, temp := range tempFiles(t, root) {
				if strings.Contains(filepath.Base(temp), "b.mp3") {
					if writeErr := os.WriteFile(temp, []byte("changed"), 0o600); writeErr != nil {
						t.Fatal(writeErr)
					}
				}
			}
		}
		return result, err
	}}
	result, err := execute.RunComponentWithTestKit(
		t.Context(),
		runRequest(
			root,
			componentFixture(
				[]reconcile.FileTuple{freezeFile(t, wav)},
				encodeOp("cmp-1", wav, first),
				encodeOp("cmp-1", wav, second),
			),
			reconcile.DesiredProfile{Encoded: &spec},
			execute.DeleteModeHard,
		),
		kit,
	)
	requireStage(t, err, execute.ComponentStageCommit)
	if !slices.Equal(result.Committed, []string{filepath.ToSlash(first)}) || len(result.Outputs) != 1 ||
		result.Outputs[0].SHA256 == "" {
		t.Fatalf("partial facts = %+v", result)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal("earlier output lost", err)
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatal("changed output published")
	}
}

func TestUnverifiedPublishProtectsTheOnlyTempCopy(t *testing.T) {
	root, wav := newAudioRoot(t)
	spec := mp3Spec()
	target := filepath.Join(root, "output.mp3")
	kit := execute.ComponentRunTestKit{Move: func(string, string) (execute.MoveResult, error) {
		return execute.MoveResult{Unverified: true}, errors.New("network reply lost")
	}}
	result, err := execute.RunComponentWithTestKit(
		t.Context(),
		runRequest(
			root,
			componentFixture([]reconcile.FileTuple{freezeFile(t, wav)}, encodeOp("cmp-1", wav, target)),
			reconcile.DesiredProfile{Encoded: &spec},
			execute.DeleteModeHard,
		),
		kit,
	)
	requireStage(t, err, execute.ComponentStageCommit)
	temps := tempFiles(t, root)
	if len(temps) != 1 || !slices.Contains(result.Recovery, filepath.ToSlash(temps[0])) || len(result.Committed) != 0 {
		t.Fatalf("unverified result lost its source: %+v, %v", result, temps)
	}
}

func TestRestoreConflictPreservesOldRecoveryAndCompetingTarget(t *testing.T) {
	root, wav := newAudioRoot(t)
	spec := mp3Spec()
	target := filepath.Join(root, "output.mp3")
	if err := os.WriteFile(target, []byte("old audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	component := componentFixture(
		[]reconcile.FileTuple{freezeFile(t, wav), freezeFile(t, target)},
		encodeOp("cmp-1", wav, target),
	)
	component.Variants = declaredReplacement(target)
	moves := filesystem.ConversionMoves{}
	kit := execute.ComponentRunTestKit{Move: func(source, destination string) (execute.MoveResult, error) {
		if strings.Contains(source, ".tmp.") {
			if err := os.WriteFile(destination, []byte("competing writer"), 0o600); err != nil {
				t.Fatal(err)
			}
			return execute.MoveResult{}, os.ErrExist
		}
		return moves.Move(source, destination)
	}}
	result, err := execute.RunComponentWithTestKit(
		t.Context(),
		runRequest(root, component, reconcile.DesiredProfile{Encoded: &spec}, execute.DeleteModeSoft),
		kit,
	)
	requireStage(t, err, execute.ComponentStageCommit)
	if string(readBytes(t, target)) != "competing writer" {
		t.Fatal("restore overwrote competing target")
	}
	found := false
	for _, path := range result.Recovery {
		if path != filepath.ToSlash(target) && string(readBytes(t, filepath.FromSlash(path))) == "old audio" {
			found = true
		}
	}
	if !found {
		t.Fatalf("old audio recovery not reported: %+v", result)
	}
}

func TestValidateFailurePreservesExternallyReplacedTemp(t *testing.T) {
	root, wav := newAudioRoot(t)
	spec := mp3Spec()
	target := filepath.Join(root, "output.mp3")
	prepared, err := execute.PrepareComponent(
		t.Context(),
		runRequest(
			root,
			componentFixture([]reconcile.FileTuple{freezeFile(t, wav)}, encodeOp("cmp-1", wav, target)),
			reconcile.DesiredProfile{Encoded: &spec},
			execute.DeleteModeHard,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if encodeErr := prepared.EncodeOne(t.Context(), 0); encodeErr != nil {
		t.Fatal(encodeErr)
	}
	temp := tempFiles(t, root)[0]
	if renameErr := os.Rename(temp, filepath.Join(root, "saved-output")); renameErr != nil {
		t.Fatal(renameErr)
	}
	if writeErr := os.WriteFile(temp, []byte("external replacement"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	result, err := prepared.Commit(t.Context())
	requireStage(t, err, execute.ComponentStageValidate)
	if string(readBytes(t, temp)) != "external replacement" ||
		!slices.Contains(result.Recovery, filepath.ToSlash(temp)) {
		t.Fatalf("foreign replacement cleaned: %+v", result)
	}
}
