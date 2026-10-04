package ffmpeg

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

// CheckFD proves this configured binary pair can seek, encode, probe and fully
// decode the real session's containers over regular-file standard handles.
func (f Encoder) CheckFD(ctx context.Context, specs []reconcile.AudioOutputSpec) (failure error) {
	dir, err := os.MkdirTemp("", "conversion-fd-")
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, os.RemoveAll(dir)) }()
	input, err := os.OpenFile(filepath.Join(dir, "input.wav"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer input.Close()
	// One second of silent mono 16-bit PCM at 48 kHz, including RIFF sizes.
	wav := make([]byte, 44+48000*2)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], 96036)
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 48000)
	binary.LittleEndian.PutUint32(wav[28:], 96000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 96000)
	if _, err := input.Write(wav); err != nil {
		return err
	}
	for i, spec := range specs {
		output, err := os.OpenFile(
			filepath.Join(dir, fmt.Sprintf("output-%d", i)),
			os.O_CREATE|os.O_EXCL|os.O_RDWR,
			0o600,
		)
		if err != nil {
			return err
		}
		_, encodeErr := f.Encode(ctx, input, output, spec)
		closeErr := output.Close()
		if err := errors.Join(encodeErr, closeErr); err != nil {
			return fmt.Errorf("FD container %s: %w", spec.Codec, err)
		}
	}
	return nil
}
