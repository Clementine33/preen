package execute_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

// An optional directory exercises a real network mount with an owned fixture;
// existing media is never modified.
func TestComponentStageFactsSurviveClose(t *testing.T) {
	root := t.TempDir()
	if parent := os.Getenv("CONVERSION_TEST_STAGE_DIR"); parent != "" {
		var err error
		root, err = os.MkdirTemp(parent, ".tmp.conversion-stage-*")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(root); err != nil {
				t.Error(err)
			}
		})
	}
	_, fixture := newAudioRoot(t)
	if realSource := os.Getenv("CONVERSION_TEST_STAGE_SOURCE"); realSource != "" {
		fixture = realSource
	}
	source := filepath.Join(root, "source.wav")
	writeBytes(t, source, readBytes(t, fixture))
	target := filepath.Join(root, "output.opus")
	spec := reconcile.AudioOutputSpec{
		Codec:   reconcile.CodecOpus,
		Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 128},
	}
	result, err := execute.RunComponent(t.Context(), runRequest(root,
		componentFixture([]reconcile.FileTuple{freezeFile(t, source)}, encodeOp("cmp-1", source, target)),
		reconcile.DesiredProfile{Encoded: &spec}, execute.DeleteModeHard))
	if err != nil {
		t.Fatalf("staged output must survive closing the encoder handle: %+v: %v", result, err)
	}
	if len(result.Outputs) != 1 || result.Outputs[0].SHA256 == "" || len(result.Committed) != 1 {
		t.Fatalf("missing published facts: %+v", result)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(readBytes(t, target))
	if result.Outputs[0].SHA256 != hex.EncodeToString(hash[:]) ||
		result.Outputs[0].Size != info.Size() || result.Outputs[0].Mtime != info.ModTime().Unix() {
		t.Fatalf("published facts do not describe the closed output: %+v", result.Outputs[0])
	}
}

// Model the server's close-time metadata finalization at the first read-only
// reopen, so CI checks the same lifecycle without requiring an SMB server.
func TestComponentCapturesFinalizedStageFacts(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "mtime finalized", true: "object replaced"}[replace], func(t *testing.T) {
			root, source := newAudioRoot(t)
			target := filepath.Join(root, "output.mp3")
			mtime := time.Unix(1700000000, 456789000)
			files := &finalizingStageHandles{finalize: func(stage string) {
				if replace {
					if err := os.Rename(stage, stage+".original"); err != nil {
						t.Fatal(err)
					}
					writeBytes(t, stage, readBytes(t, stage+".original"))
				}
				if err := os.Chtimes(stage, mtime, mtime); err != nil {
					t.Fatal(err)
				}
			}}
			spec := mp3Spec()
			req := runRequest(root,
				componentFixture([]reconcile.FileTuple{freezeFile(t, source)}, encodeOp("cmp-1", source, target)),
				reconcile.DesiredProfile{Encoded: &spec}, execute.DeleteModeHard)
			req.Files = files
			result, err := execute.RunComponent(t.Context(), req)
			if !files.reopened {
				t.Fatal("staged facts were captured before writable handle closure")
			}
			if replace {
				requireStage(t, err, execute.ComponentStageMaterialize)
				if len(result.Committed) != 0 || len(result.Outputs) != 0 {
					t.Fatalf("published a replaced object: %+v", result)
				}
				return
			}
			if err != nil || len(result.Outputs) != 1 || result.Outputs[0].Mtime != mtime.Unix() {
				t.Fatalf("finalized facts were not retained: %+v, %v", result, err)
			}
		})
	}
}

type finalizingStageHandles struct {
	filesystem.ConversionHandles

	writer   *os.File
	stage    string
	finalize func(string)
	reopened bool
}

func (f *finalizingStageHandles) CreateStage(root, stage string) (*os.File, error) {
	file, err := f.ConversionHandles.CreateStage(root, stage)
	f.writer, f.stage = file, stage
	return file, err
}

func (f *finalizingStageHandles) OpenSource(root, source string) (*os.File, error) {
	if source == f.stage {
		if _, err := f.writer.Stat(); !errors.Is(err, os.ErrClosed) {
			return nil, errors.New("writer still open at final-facts capture")
		}
		f.reopened = true
		f.finalize(source)
	}
	return f.ConversionHandles.OpenSource(root, source)
}
