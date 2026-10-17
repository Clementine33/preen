package execute

import (
	"context"
	"os"

	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

// AudioStream is the probed shape of one audio stream: what the pipeline reads
// from the handles used by an encoding worker.
type AudioStream struct {
	Codec      string `json:"codec_name"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
	Bits       int    `json:"bits_per_sample"`
	RawBits    string `json:"bits_per_raw_sample"`
	Duration   string `json:"duration"`
}

// ValidatedOutput describes the final read-only object, including the digest
// used by lossy generation credentials. Lossless outputs have no digest.
type ValidatedOutput struct {
	Stream AudioStream
	SHA256 string
}

// Encoder is the media tool the component pipeline is written against: it
// probes files and writes one frozen target specification. It is a port, not a
// command — everything ffmpeg is made of (tool paths, arguments, the checks a
// written file passes) belongs to the adapter that implements it.
type Encoder interface {
	// Encode uses independently opened, seekable handles and returns source
	// facts. The caller closes the writer before validating the final object.
	Encode(ctx context.Context, src, dst *os.File, spec reconcile.AudioOutputSpec) (AudioStream, error)
	// ValidateOutput probes and checks a read-only output against its source.
	// Handles remain owned by the caller; sequential operations reset the cursor.
	ValidateOutput(context.Context, *os.File, reconcile.AudioOutputSpec, AudioStream) (ValidatedOutput, error)
	// Check reports both tools being available before encoding.
	Check() error
	// CheckFD probes actual output containers once per encoding session.
	CheckFD(context.Context, []reconcile.AudioOutputSpec) error
	// Probe reads the first audio stream of one file.
	Probe(ctx context.Context, path string) (AudioStream, error)
	// Version reports the encoder's own version line, the diagnostic a
	// generation credential records; an unavailable tool reports an empty
	// string rather than failing the generation it describes.
	Version(ctx context.Context) string
	// TargetFacts names the encoder binary and the rate-control mode one
	// encoded target is written with: what a generation credential records, and
	// what makes a record written under other settings no evidence for this one.
	TargetFacts(spec reconcile.AudioOutputSpec) (encoder, mode string, err error)
}

// EncoderFactory supplies an encoder whose tool paths belong to the adapter.
// Conversion captures one encoder per execution session.
type EncoderFactory func() Encoder
