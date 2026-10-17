package conversion_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	appconfig "github.com/onsei/organizer/backend/internal/adapters/settings"
	"github.com/onsei/organizer/backend/internal/conversion"
	"github.com/onsei/organizer/backend/internal/conversion/execute"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/workset"
)

func TestExecutionMemoryFingerprintUsesWholeSecondsAndSelectedRoots(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "selected")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "track.FLAC")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Unix(1700000000, 987654321)
	if err := os.Chtimes(file, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	digest, count := reconcile.InventoryFingerprint(
		[]reconcile.AudioEntry{{PathPosix: filepath.ToSlash(file), Size: 5, Mtime: mtime.Unix()}},
	)
	var walked, refreshed int
	task := conversion.New(
		nil,
		appconfig.NewReader(t.TempDir()),
		func() execute.Encoder { return ffmpeg.New(ffmpeg.ToolsConfig{}) },
		nil,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{
			Walk: func(ctx context.Context, roots []string, base string, emit func(string, inventory.DirEntry) error) error {
				walked++
				return filesystem.WalkSelectedEntriesParallel(ctx, roots, base, 32, emit)
			},
			Moves: filesystem.ConversionMoves{},
			Files: filesystem.ConversionHandles{},
		},
	)
	in := workset.ExecutionSessionInput{WorksetRoot: base, Options: json.RawMessage(`{"delete_mode":"hard"}`),
		Units:    []workset.ExecutionUnit{{Index: 0, RootPath: filepath.ToSlash(root), Payload: json.RawMessage(`{}`)}},
		Outcomes: map[int]json.RawMessage{0: json.RawMessage(`{"operations":[]}`)},
		Detail: &workset.PlanDetail{Roots: []workset.PlanRootRecord{
			{RootPath: filepath.ToSlash(root), InventoryFingerprint: digest, EntryCount: count},
			{RootPath: filepath.Join(base, "unselected-missing"), RootStatus: "missing"},
		}}, Refresh: func(context.Context, []string) error { refreshed++; return errors.New("refresh failed") }}
	if _, err := task.NewExecutionSession(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if walked != 1 || refreshed != 0 {
		t.Fatalf("walks %d, refreshes %d", walked, refreshed)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	_, err := task.NewExecutionSession(t.Context(), in)
	failure, ok := workset.AsError(err)
	if !ok || failure.Code != "INPUT_CHANGED" || !strings.Contains(failure.Message, "refresh failed") {
		t.Fatalf("failure = %v", err)
	}
	if refreshed != 1 {
		t.Fatalf("refreshes = %d", refreshed)
	}
	in.Units = nil
	if _, err := task.NewExecutionSession(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if walked != 2 || refreshed != 1 {
		t.Fatal("empty execution scanned or refreshed")
	}
}

func TestExecutionMissingAndIOErrorsKeepTheirCodes(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		walkErr error
		code    string
	}{
		{"missing", os.ErrNotExist, "INPUT_CHANGED"}, {"permission", os.ErrPermission, "SCAN_FAILED"}, {"IO", errors.New("IO failure"), "SCAN_FAILED"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			task := conversion.New(nil, nil, nil, nil, nil, nil, conversion.ExecutionIO{
				Walk: func(context.Context, []string, string, func(string, inventory.DirEntry) error) error {
					return scenario.walkErr
				},
			})
			_, err := task.NewExecutionSession(
				t.Context(),
				workset.ExecutionSessionInput{
					Units:    []workset.ExecutionUnit{{RootPath: "/missing", Payload: json.RawMessage(`{}`)}},
					Outcomes: map[int]json.RawMessage{0: json.RawMessage(`{"operations":[]}`)},
					Detail:   &workset.PlanDetail{},
					Refresh:  func(context.Context, []string) error { return errors.New("refresh failed") },
				},
			)
			failure, ok := workset.AsError(err)
			if !ok || failure.Code != scenario.code {
				t.Fatalf("error = %v, want %s", err, scenario.code)
			}
		})
	}
}
