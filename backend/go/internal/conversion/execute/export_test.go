package execute

import (
	"context"
	"os"

	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

// ComponentRunTestKit replaces component-run IO in external tests so failures
// can be injected at specific filesystem boundaries; a nil field keeps the
// real implementation.
type ComponentRunTestKit struct {
	Encode   func(ctx context.Context, src, dst string, spec reconcile.AudioOutputSpec) error
	Validate func(context.Context, *os.File, reconcile.AudioOutputSpec, AudioStream) (ValidatedOutput, error)
	Rename   func(oldpath, newpath string) error
	Remove   func(path string) error
	Move     func(string, string) (MoveResult, error)
}

// PrepareComponentWithTestKit prepares one component with replaced IO, the
// shape a concurrent session drives.
func PrepareComponentWithTestKit(
	ctx context.Context, req ComponentRunRequest, kit ComponentRunTestKit,
) (*PreparedComponent, error) {
	return prepareComponent(ctx, req, testToolkit(req, kit))
}

// RunComponentWithTestKit runs the exported component entry with replaced IO.
func RunComponentWithTestKit(
	ctx context.Context, req ComponentRunRequest, kit ComponentRunTestKit,
) (ComponentRunResult, error) {
	return runComponent(ctx, req, testToolkit(req, kit))
}

// testToolkit is the component IO surface of one test kit.
func testToolkit(req ComponentRunRequest, kit ComponentRunTestKit) *componentToolkit {
	tk := defaultComponentToolkit(req)
	if req.Recover != nil {
		tk.recoverMove = func(ctx context.Context, root, src string) (string, MoveResult, error) {
			dst, err := req.Recover(ctx, root, src)
			return dst, MoveResult{TargetCreated: err == nil, SourceRemoved: err == nil}, err
		}
	}
	if kit.Encode != nil {
		tk.encode = func(ctx context.Context, src, dst *os.File, spec reconcile.AudioOutputSpec) (AudioStream, error) {
			if err := kit.Encode(ctx, src.Name(), dst.Name(), spec); err != nil {
				return AudioStream{}, err
			}
			stream, err := req.Encoder.Probe(context.WithoutCancel(ctx), src.Name())
			return stream, err
		}
	}
	if kit.Validate != nil {
		tk.validate = kit.Validate
	}
	if kit.Rename != nil {
		tk.move = func(src, dst string) (MoveResult, error) {
			err := kit.Rename(src, dst)
			return MoveResult{TargetCreated: err == nil, SourceRemoved: err == nil}, err
		}
	}
	if kit.Move != nil {
		tk.move = kit.Move
	}
	if kit.Remove != nil {
		tk.remove = kit.Remove
	}
	return tk
}
