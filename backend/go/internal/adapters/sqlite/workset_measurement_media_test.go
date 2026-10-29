//nolint:testpackage // These simulated dependencies belong to the private-driver measurement harness.
package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"sync/atomic"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

// Encoding/probing has no artificial delay. The real execution pipeline still
// opens, fsyncs, validates identities, publishes files and persists credentials.
type worksetMeasurementEncoder struct{}

func (worksetMeasurementEncoder) Encode(
	_ context.Context,
	src, dst *os.File,
	_ reconcile.AudioOutputSpec,
) (execute.AudioStream, error) {
	_, err := io.Copy(dst, src)
	return execute.AudioStream{Codec: "flac", SampleRate: "48000", Channels: 2, Duration: "1"}, err
}

func (worksetMeasurementEncoder) ValidateOutput(
	_ context.Context,
	file *os.File,
	_ reconcile.AudioOutputSpec,
	_ execute.AudioStream,
) (execute.ValidatedOutput, error) {
	hash := sha256.New()
	_, err := io.Copy(hash, file)
	return execute.ValidatedOutput{
		Stream: execute.AudioStream{Codec: "mp3"},
		SHA256: hex.EncodeToString(hash.Sum(nil)),
	}, err
}
func (worksetMeasurementEncoder) Check() error { return nil }
func (worksetMeasurementEncoder) CheckFD(context.Context, []reconcile.AudioOutputSpec) error {
	return nil
}
func (worksetMeasurementEncoder) Probe(context.Context, string) (execute.AudioStream, error) {
	return execute.AudioStream{Codec: "flac", SampleRate: "48000", Channels: 2, Duration: "1"}, nil
}
func (worksetMeasurementEncoder) Version(context.Context) string { return "synthetic encoder" }
func (worksetMeasurementEncoder) TargetFacts(reconcile.AudioOutputSpec) (string, string, error) {
	return "synthetic", "cbr", nil
}

// Each member has a distinct work and cover demand; network retrieval is replaced
// at the existing ports, without modifying metadata or delivery persistence.
type worksetMeasurementSource struct{ version *atomic.Uint32 }

func (worksetMeasurementSource) FetchStatic(_ context.Context, id works.WorkCode, _ string) (refresh.Snapshot, error) {
	title := string(id)
	return refresh.Snapshot{
		Core: &works.ImportCore{Source: "dlsite", FetchedAt: "2026-10-30T00:00:00.000000000Z", Title: &title},
		Editions: &works.ImportEditions{
			Source:    "dlsite",
			FetchedAt: "2026-10-30T00:00:00.000000000Z",
		},
		MainURL: "https://img.dlsite.jp/measurement.png",
	}, nil
}

func (worksetMeasurementSource) FetchDynamic(
	_ context.Context,
	ids []works.WorkCode,
	_ string,
) (map[works.WorkCode]works.ImportRequest, error) {
	result := make(map[works.WorkCode]works.ImportRequest, len(ids))
	for _, id := range ids {
		result[id] = works.ImportRequest{
			Statistics: &works.ImportStatistics{FetchedAt: "2026-10-30T00:00:00.000000000Z"},
			Relations: &works.ImportRelations{
				Source:    "dlsite",
				FetchedAt: "2026-10-30T00:00:00.000000000Z",
				Role:      "original",
			},
		}
	}
	return result, nil
}
func (worksetMeasurementSource) FetchASMR(context.Context, works.WorkCode, string) (refresh.Snapshot, error) {
	return refresh.Snapshot{}, fmt.Errorf("synthetic source should not need backup")
}
func (worksetMeasurementSource) FetchMain(context.Context, string, string, string) (string, error) {
	return "https://img.dlsite.jp/measurement.png", nil
}
func (s worksetMeasurementSource) Download(context.Context, string) (coverdelivery.Download, error) {
	var output bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: uint8(s.version.Load() % 256), A: 255})
	err := png.Encode(&output, img)
	return coverdelivery.Download{Data: output.Bytes(), Extension: ".png", ContentType: "image/png"}, err
}
