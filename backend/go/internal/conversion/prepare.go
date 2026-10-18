package conversion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// PrepareUnit prechecks one frozen unit through the Component pipeline and
// returns it ready to encode: the frozen outcome, profile and session options
// are decoded, and nothing is written yet.
func (t *Task) PrepareUnit(
	ctx context.Context,
	in workset.UnitRunInput,
) (workset.PreparedUnit, error) {
	component, _, err := t.prepareComponent(ctx, in)
	if err != nil {
		return nil, err
	}
	return &preparedUnit{component: component, encoder: t.encoder()}, nil
}

// prepareComponent decodes the frozen unit payloads and prechecks the
// component; a component failure is mapped onto the seam error, keeping the
// stage and code its report entry carries. The decoded profile comes back with
// it: it is what the credentials of the committed outputs are written from.
func (t *Task) prepareComponent(
	ctx context.Context,
	in workset.UnitRunInput,
) (*execute.PreparedComponent, reconcile.DesiredProfile, error) {
	var outcome reconcile.ComponentOutcome
	if err := json.Unmarshal(in.Outcome, &outcome); err != nil {
		return nil, reconcile.DesiredProfile{}, workset.NewError(
			workset.ErrKindInternal, "REVISION_LOAD_FAILED", "frozen component is unreadable", err,
		)
	}
	var profile reconcile.DesiredProfile
	if err := json.Unmarshal(in.Unit.Payload, &profile); err != nil {
		return nil, reconcile.DesiredProfile{}, workset.NewError(
			workset.ErrKindInternal, "REQUEST_LOAD_FAILED", "frozen unit is unreadable", err,
		)
	}
	mode, modeErr := deleteModeFromOptions(in.Options)
	if modeErr != nil {
		return nil, reconcile.DesiredProfile{}, modeErr
	}
	component, prepErr := execute.PrepareComponent(ctx, execute.ComponentRunRequest{
		Root:       in.Unit.RootPath,
		Component:  outcome,
		Specs:      profile,
		DeleteMode: mode,
		Encoder:    t.encoder(),
		WithinRoot: t.withinRoot,
		Moves:      t.moves,
		Files:      t.files,
	})
	if prepErr != nil {
		return nil, reconcile.DesiredProfile{}, unitFailureOf(prepErr)
	}
	return component, profile, nil
}

// preparedUnit is one prepared conversion unit: the frozen inputs, the profile
// its outputs are written for, and the component whose staged outputs the
// session encodes and commits.
type preparedUnit struct {
	component *execute.PreparedComponent
	encoder   execute.Encoder
}

// EncodeTasks is how many staged outputs Commit expects.
func (p *preparedUnit) EncodeTasks() int { return p.component.Encodes() }

// EncodeTask materializes one staged output.
func (p *preparedUnit) EncodeTask(ctx context.Context, index int) error {
	if err := p.component.EncodeOne(ctx, index); err != nil {
		return unitFailureOf(err)
	}
	return nil
}

// Commit lands the encoded outputs in frozen order and reports the unit's
// observed facts plus the inventory refresh the generic side applies.
func (p *preparedUnit) Commit(ctx context.Context) (workset.UnitResult, error) {
	res, err := p.component.Commit(ctx)
	result := p.resultOf(res, err)
	p.recordGenerations(ctx, &result, res.Outputs)
	return result, nil
}

// Discard cleans the staged outputs of a unit that will not commit.
func (p *preparedUnit) Discard(cause error) workset.UnitResult {
	return p.resultOf(p.component.Discard(cause), cause)
}

// resultOf maps one component outcome onto the seam facts. The observing run's
// cause carries the stage and code of a failed or canceled unit.
func (p *preparedUnit) resultOf(res execute.ComponentRunResult, cause error) workset.UnitResult {
	result := workset.UnitResult{
		Committed: nonNil(res.Committed),
		Removed:   nonNil(res.Removed),
		Remaining: nonNil(res.Remaining),
		Recovery:  nonNil(res.Recovery),
	}
	if res.Unverified {
		result.InventoryError = "file move outcome unverified; refresh inventory before generating another plan"
	}
	if cause != nil {
		result.Stage, result.ErrorCode, result.ErrorMessage = componentErrorOf(cause)
	}
	if res.Status == execute.ComponentStatusCanceled ||
		result.ErrorCode == execute.ComponentCodeCanceled {
		result.Canceled = true
		result.ErrorMessage = "component run canceled"
	}
	result.InventoryRemoved = result.Removed
	for _, output := range res.Outputs {
		result.InventoryRefreshed = append(
			result.InventoryRefreshed,
			inventory.InventoryFile{Path: output.Path, Size: output.Size, Mtime: output.Mtime},
		)
	}
	return result
}

// unitFailureOf maps a component failure onto the seam error: the unit's stage
// and code cross the seam so its own report entry carries them.
func unitFailureOf(err error) error {
	stage, code, message := componentErrorOf(err)
	return workset.NewError(workset.ErrKindInternal, code, message, err).WithStage(stage)
}

// recordGenerations uses the encode worker's captured hash and file facts for
// successful publications. The session's fixed tool version and target settings
// complete the credentials without reopening a committed file.
func (p *preparedUnit) recordGenerations(
	ctx context.Context,
	result *workset.UnitResult,
	outputs []execute.OutputFacts,
) {
	version := ""
	for _, output := range outputs {
		spec := output.Spec
		if spec.Lossless() || spec.Quality == nil {
			continue
		}
		if version == "" {
			version = p.encoder.Version(ctx)
		}
		encoder, mode, err := p.encoder.TargetFacts(spec)
		if err != nil {
			result.InventoryError = fmt.Sprintf("generation facts for %s: %v", output.Path, err)
			return
		}
		result.Generated = append(result.Generated, inventory.GenerationRecord{Path: output.Path,
			Codec: string(spec.Codec), Encoder: encoder, EncoderVersion: version, BitrateKbps: spec.Quality.Bitrate,
			Mode: mode, Size: output.Size, Mtime: output.Mtime, ContentSHA256: output.SHA256, CreatedAt: time.Now()})
	}
}

// nonNil turns a nil path slice into an empty one so a persisted component
// result never marshals a planned-empty list as JSON null.
func nonNil(paths []string) []string {
	if paths == nil {
		return []string{}
	}
	return paths
}
