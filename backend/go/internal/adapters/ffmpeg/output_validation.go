package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

// ValidateOutput checks the final read-only object. It never publishes or owns
// handles; the execution worker brackets this operation with identity checks.
func (f Encoder) ValidateOutput(
	ctx context.Context, file *os.File, spec reconcile.AudioOutputSpec, input execute.AudioStream,
) (execute.ValidatedOutput, error) {
	if file == nil {
		return execute.ValidatedOutput{}, fmt.Errorf("output handle required")
	}
	data, err := f.probeData(ctx, file, false, true)
	if err != nil {
		return execute.ValidatedOutput{}, err
	}
	output, err := probeStream(data)
	if err != nil {
		return execute.ValidatedOutput{}, err
	}
	if comparisonErr := compareOutput(input, output, spec); comparisonErr != nil {
		return execute.ValidatedOutput{}, comparisonErr
	}
	if spec.Codec == reconcile.CodecOpus {
		digest, pageErr := hashOpus(ctx, file)
		return execute.ValidatedOutput{Stream: output, SHA256: digest}, pageErr
	}
	if decodeErr := f.decodeHandle(ctx, file); decodeErr != nil {
		return execute.ValidatedOutput{}, decodeErr
	}
	result := execute.ValidatedOutput{Stream: output}
	if !spec.Lossless() {
		result.SHA256, err = hashOutput(ctx, file)
	}
	return result, err
}

func compareOutput(input, output execute.AudioStream, spec reconcile.AudioOutputSpec) error {
	target, err := audioEncodingArgs(input, spec)
	if err != nil {
		return err
	}
	if output.Codec != target.Codec {
		return fmt.Errorf("unexpected output codec: %s", output.Codec)
	}
	inDuration, inErr := strconv.ParseFloat(input.Duration, 64)
	outDuration, outErr := strconv.ParseFloat(output.Duration, 64)
	if inErr != nil || outErr != nil || math.IsNaN(inDuration) || math.IsNaN(outDuration) ||
		math.IsInf(inDuration, 0) || math.IsInf(outDuration, 0) || inDuration <= 0 || outDuration <= 0 ||
		math.Abs(inDuration-outDuration) > 0.1 {
		return fmt.Errorf("output duration does not match source")
	}
	if output.Channels != input.Channels {
		return fmt.Errorf("output changed channel count")
	}
	if output.SampleRate != input.SampleRate && spec.Codec != reconcile.CodecOpus {
		return fmt.Errorf("output changed sample rate")
	}
	return nil
}

// hashOutput records the observed original bytes; it is not an expected checksum.
func hashOutput(ctx context.Context, file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	hash := sha256.New()
	buffer := make([]byte, 256*1024)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
