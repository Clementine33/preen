package execute_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

func TestStagedOutputMtimeIsObservedOnly(t *testing.T) {
	root, source := newAudioRoot(t)
	target := filepath.Join(root, "output.mp3")
	spec := mp3Spec()
	encoder := ffmpeg.New(ffmpeg.ToolsConfig{})
	var stage string
	kit := execute.ComponentRunTestKit{Validate: func(
		ctx context.Context, file *os.File, spec reconcile.AudioOutputSpec, input execute.AudioStream,
	) (execute.ValidatedOutput, error) {
		validated, err := encoder.ValidateOutput(ctx, file, spec, input)
		if err != nil {
			return validated, err
		}
		stage = file.Name()
		mtime := time.Unix(1700000000, 161000000)
		return validated, os.Chtimes(stage, mtime, mtime)
	}}
	prepared, err := execute.PrepareComponentWithTestKit(t.Context(), runRequest(root,
		componentFixture([]reconcile.FileTuple{freezeFile(t, source)}, encodeOp("cmp-1", source, target)),
		reconcile.DesiredProfile{Encoded: &spec}, execute.DeleteModeHard), kit)
	if err != nil {
		t.Fatal(err)
	}
	if encodeErr := prepared.EncodeOne(t.Context(), 0); encodeErr != nil {
		prepared.Discard(encodeErr)
		t.Fatalf("mtime update during validation rejected: %v", encodeErr)
	}
	mtime := time.Unix(1700000002, 789000000)
	if timeErr := os.Chtimes(stage, mtime, mtime); timeErr != nil {
		t.Fatal(timeErr)
	}
	result, err := prepared.Commit(t.Context())
	if err != nil || len(result.Committed) != 1 || len(result.Outputs) != 1 {
		t.Fatalf("mtime update before publication rejected: %+v, %v", result, err)
	}
	if result.Outputs[0].Mtime != mtime.Unix() || result.Outputs[0].SHA256 == "" {
		t.Fatalf("missing observed publication facts: %+v", result.Outputs[0])
	}
}

func TestComponentRejectsOutputSizeChangeDuringValidation(t *testing.T) {
	root, source := newAudioRoot(t)
	target := filepath.Join(root, "output.mp3")
	spec := mp3Spec()
	encoder := ffmpeg.New(ffmpeg.ToolsConfig{})
	kit := execute.ComponentRunTestKit{Validate: func(
		ctx context.Context, file *os.File, spec reconcile.AudioOutputSpec, input execute.AudioStream,
	) (execute.ValidatedOutput, error) {
		validated, err := encoder.ValidateOutput(ctx, file, spec, input)
		if err != nil {
			return validated, err
		}
		return validated, os.Truncate(file.Name(), 0)
	}}
	result, err := execute.RunComponentWithTestKit(t.Context(), runRequest(root,
		componentFixture([]reconcile.FileTuple{freezeFile(t, source)}, encodeOp("cmp-1", source, target)),
		reconcile.DesiredProfile{Encoded: &spec}, execute.DeleteModeHard), kit)
	requireStage(t, err, execute.ComponentStageMaterialize)
	if errorCode(t, err) != execute.ComponentCodeValidateFailed || len(result.Committed) != 0 ||
		len(result.Outputs) != 0 || len(tempFiles(t, root)) != 0 {
		t.Fatalf("size change did not stop publication and clean stage: %+v, %v", result, err)
	}
}
