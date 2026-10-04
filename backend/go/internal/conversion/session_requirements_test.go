package conversion_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	appconfig "github.com/onsei/organizer/backend/internal/adapters/settings"
	"github.com/onsei/organizer/backend/internal/conversion"
	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/workset"
)

type sessionMoves struct {
	execute.FileMoves

	calls        int
	failure      error
	requirements []execute.MoveRequirement
}

func (m *sessionMoves) Probe(_ context.Context, requirements []execute.MoveRequirement) error {
	m.calls++
	m.requirements = requirements
	return m.failure
}

type sessionTools struct {
	execute.Encoder

	checks, versions, fdChecks int
	specs                      []reconcile.AudioOutputSpec
}

func (e *sessionTools) Check() error                   { e.checks++; return nil }
func (e *sessionTools) Version(context.Context) string { e.versions++; return "fixed-version" }
func (e *sessionTools) CheckFD(_ context.Context, specs []reconcile.AudioOutputSpec) error {
	e.fdChecks++
	e.specs = specs
	return nil
}

func sessionInput(t *testing.T, encode bool) workset.ExecutionSessionInput {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "work")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.wav")
	if err := os.WriteFile(source, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	tuple := reconcile.FileTuple{Path: filepath.ToSlash(source), Size: info.Size(), Mtime: info.ModTime().Unix()}
	op := reconcile.Operation{
		ComponentID: "component",
		SourcePath:  tuple.Path,
		Kind:        reconcile.OpKindRemoveObsolete,
		Phase:       reconcile.PhaseRemoveObsoleteAudio,
	}
	profile := reconcile.DesiredProfile{}
	if encode {
		op.Kind, op.Phase, op.TargetPath = reconcile.OpKindEncode, reconcile.PhaseMaterializeOutputs, filepath.ToSlash(
			filepath.Join(root, "output.mp3"),
		)
		profile.Encoded = &reconcile.AudioOutputSpec{
			Codec:   reconcile.CodecMp3,
			Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 192},
		}
	}
	outcome, err := json.Marshal(
		reconcile.ComponentOutcome{
			ComponentID: "component",
			Status:      reconcile.StatusOK,
			Files:       []reconcile.FileTuple{tuple},
			Operations:  []reconcile.Operation{op},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	digest, count := reconcile.InventoryFingerprint(
		[]reconcile.AudioEntry{{PathPosix: tuple.Path, Size: tuple.Size, Mtime: tuple.Mtime}},
	)
	return workset.ExecutionSessionInput{
		WorksetRoot: base,
		Options:     json.RawMessage(`{"delete_mode":"soft"}`),
		Units:       []workset.ExecutionUnit{{Index: 0, RootPath: filepath.ToSlash(root), Payload: payload}},
		Outcomes:    map[int]json.RawMessage{0: outcome},
		Detail: &workset.PlanDetail{
			Roots: []workset.PlanRootRecord{
				{RootPath: filepath.ToSlash(root), InventoryFingerprint: digest, EntryCount: count},
			},
		},
	}
}

func requirementsTask(t *testing.T, moves *sessionMoves, build execute.EncoderFactory) *conversion.Task {
	t.Helper()
	return conversion.New(
		nil,
		appconfig.NewReader(t.TempDir()),
		build,
		nil,
		nil,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{
			Moves: moves,
			Files: filesystem.ConversionHandles{},
			Walk: func(ctx context.Context, roots []string, base string, emit func(string, inventory.DirEntry) error) error {
				return filesystem.WalkSelectedEntriesParallel(ctx, roots, base, 32, emit)
			},
		},
	)
}

func TestSessionFixesToolsOnceAndPureDeleteDoesNotBuildThem(t *testing.T) {
	moves, tools := &sessionMoves{}, &sessionTools{}
	builds := 0
	task := requirementsTask(t, moves, func(execute.ToolsConfig) execute.Encoder { builds++; return tools })
	in := sessionInput(t, true)
	preparer, err := task.NewExecutionSession(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, prepareErr := preparer.PrepareUnit(
			t.Context(),
			workset.UnitRunInput{Unit: in.Units[0], Outcome: in.Outcomes[0], Options: in.Options},
		); prepareErr != nil {
			t.Fatal(prepareErr)
		}
	}
	if builds != 1 || tools.checks != 1 || tools.versions != 1 || tools.fdChecks != 1 || len(tools.specs) != 1 {
		t.Fatalf("session dependencies: builds %d, tools %+v", builds, tools)
	}
	for _, mode := range []string{"hard", "soft"} {
		in := sessionInput(t, false)
		in.Options = json.RawMessage(`{"delete_mode":"` + mode + `"}`)
		if _, deleteErr := task.NewExecutionSession(t.Context(), in); deleteErr != nil {
			t.Fatal(deleteErr)
		}
	}
	if builds != 1 || moves.calls != 2 || len(moves.requirements) != 1 || !moves.requirements[0].Recovery {
		t.Fatalf("delete unexpectedly used encoder or recovery probe: builds %d moves %+v", builds, moves)
	}
}

func TestFilesystemProbeFailurePrecedesToolConstruction(t *testing.T) {
	for _, scenario := range []struct {
		failure error
		code    string
	}{{execute.ErrMoveUnsupported, "FILESYSTEM_OPERATION_UNSUPPORTED"}, {os.ErrPermission, "FILESYSTEM_PRECHECK_FAILED"}, {errors.New("IO failure"), "FILESYSTEM_PRECHECK_FAILED"}} {
		moves := &sessionMoves{failure: scenario.failure}
		task := requirementsTask(t, moves, func(execute.ToolsConfig) execute.Encoder {
			t.Fatal("tools built before successful filesystem probe")
			return nil
		})
		_, err := task.NewExecutionSession(t.Context(), sessionInput(t, true))
		failure, ok := workset.AsError(err)
		if !ok || failure.Code != scenario.code {
			t.Fatalf("failure = %v", err)
		}
	}
}

func TestSessionCapturesConfigurationBeforeInputWalk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeConfig := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`{"tools":{"ffmpeg_path":"`+value+`"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig("first-tool")
	var configs []execute.ToolsConfig
	task := conversion.New(nil, appconfig.NewReader(dir), func(config execute.ToolsConfig) execute.Encoder {
		configs = append(configs, config)
		return &sessionTools{}
	}, nil, nil, filesystem.IsResolvedWithinRoot, conversion.ExecutionIO{
		Moves: &sessionMoves{},
		Files: filesystem.ConversionHandles{},
		Walk: func(ctx context.Context, roots []string, base string, emit func(string, inventory.DirEntry) error) error {
			writeConfig("next-tool")
			return filesystem.WalkSelectedEntriesParallel(ctx, roots, base, 32, emit)
		},
	})
	in := sessionInput(t, true)
	for range 2 {
		if _, err := task.NewExecutionSession(t.Context(), in); err != nil {
			t.Fatal(err)
		}
	}
	if len(configs) != 2 || configs[0].FFmpegPath != "first-tool" || configs[1].FFmpegPath != "next-tool" {
		t.Fatalf("configuration was not frozen at session entry: %+v", configs)
	}
}
