package ffmpeg

import (
	"bytes"
	"context"
	"os/exec"
)

var ProbeHandleForTest = Encoder.probeHandle
var DecodeHandleForTest = Encoder.decodeHandle

func ValidationCommandForTest(ctx context.Context, cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return validationCommand(ctx, cmd, &stderr)
}

var HashOpusForTest = hashOpus
