package conversion_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	appconfig "github.com/onsei/organizer/backend/internal/adapters/settings"
	"github.com/onsei/organizer/backend/internal/conversion"
	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
	"github.com/onsei/organizer/backend/internal/workset"
)

type diagnosticEncoder struct{ execute.Encoder }

func (diagnosticEncoder) ValidateOutput(
	context.Context, *os.File, reconcile.AudioOutputSpec, execute.AudioStream,
) (execute.ValidatedOutput, error) {
	return execute.ValidatedOutput{}, errors.New("decode output: validation diagnostic: damaged packet")
}

func TestConversionValidationFailureKeepsDiagnostic(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source.wav"), filepath.Join(root, "target.m4a")
	runFFmpeg(t, "-f", "lavfi", "-i", "sine=duration=0.2:sample_rate=48000", "-c:a", "pcm_s24le", source)
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	spec := reconcile.AudioOutputSpec{Codec: reconcile.CodecAac,
		Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 128}}
	outcome := reconcile.ComponentOutcome{ComponentID: "cmp", Status: reconcile.StatusOK,
		Files: []reconcile.FileTuple{{Path: filepath.ToSlash(source), Size: info.Size(), Mtime: info.ModTime().Unix()}},
		Operations: []reconcile.Operation{{ComponentID: "cmp", Kind: reconcile.OpKindEncode,
			Phase:      reconcile.PhaseMaterializeOutputs,
			SourcePath: filepath.ToSlash(source),
			TargetPath: filepath.ToSlash(target),
		}}}
	task := conversion.New(nil, appconfig.NewReader(root), func(cfg execute.ToolsConfig) execute.Encoder {
		return diagnosticEncoder{ffmpeg.New(cfg)}
	}, nil, nil, filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}})
	prepared, err := task.PrepareUnit(t.Context(), workset.UnitRunInput{
		Unit:    workset.ExecutionUnit{RootPath: root, Payload: jsonBytes(t, reconcile.DesiredProfile{Encoded: &spec})},
		Outcome: jsonBytes(t, outcome),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = prepared.EncodeTask(t.Context(), 0)
	result := prepared.Discard(err)
	failure, ok := workset.AsError(err)
	if !ok || failure.Code != execute.ComponentCodeValidateFailed ||
		failure.Stage != execute.ComponentStageMaterialize {
		t.Fatalf("failure: %v", err)
	}
	for _, message := range []string{failure.Message, result.ErrorMessage} {
		if !strings.Contains(message, "damaged packet") || !strings.Contains(message, filepath.ToSlash(target)) {
			t.Fatalf("validation cause or target omitted: %s", message)
		}
	}
}
