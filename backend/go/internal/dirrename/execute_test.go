package dirrename_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/dirrename"
	"github.com/onsei/organizer/backend/internal/workset"
)

func detailRoot(m *workset.WorksetMember) string {
	return filepath.ToSlash(filepath.Dir(filepath.FromSlash(m.FolderPath)))
}

func plannedUnit(
	t *testing.T,
	task *dirrename.Task,
	m *workset.WorksetMember,
) (workset.ExecutionUnit, *workset.PlanDetail) {
	t.Helper()
	draft, _, _ := task.SeedDraft(nil)
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: detailRoot(m), RawDraft: draft, Members: []*workset.WorksetMember{m}},
	)
	if err != nil {
		t.Fatal(err)
	}
	d := detail(plan)
	frozen, reasons, err := task.FreezeExecution(context.Background(), workset.RevisionFacts{Detail: d})
	if err != nil || len(reasons) > 0 {
		t.Fatalf("freeze: %v %v", reasons, err)
	}
	return frozen.Units[0], d
}
func TestRenameCommitsWithoutScanningAndKeepsDirectoryContents(t *testing.T) {
	repo, task, m := fixture(t, "title")
	unit, d := plannedUnit(t, task, m)
	if err := os.WriteFile(filepath.Join(m.FolderPath, "private"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	preparer, err := task.NewExecutionSession(
		t.Context(),
		workset.ExecutionSessionInput{WorksetRoot: detailRoot(m), Units: []workset.ExecutionUnit{unit}, Detail: d},
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := preparer.PrepareUnit(t.Context(), workset.UnitRunInput{WorksetRoot: detailRoot(m), Unit: unit})
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Commit(t.Context())
	if err != nil || result.ErrorCode != "" || *result.CompletedOperations != 1 {
		t.Fatalf("commit: %+v %v", result, err)
	}
	var facts dirrename.Result
	if err = json.Unmarshal(result.TaskResult, &facts); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filepath.Join(facts.TargetPath, "private"))
	if err != nil || string(bytes) != "kept" {
		t.Fatalf("content: %s %v", bytes, err)
	}
	copies, err := repo.CoverTargets(t.Context(), detailRoot(m), []string{facts.TargetPath})
	if err != nil || len(copies) != 1 {
		t.Fatalf("binding lost: %v %v", copies, err)
	}
}
func TestRenameSyncFailureRestoresOnlyCurrentDirectoryAndReportsConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "rollback conflict"}[conflict], func(t *testing.T) {
			repo, _, m := fixture(t, "title")
			task := dirrename.New(repo, filesystem.ReadRenameDirectory, dirrename.ExecutionIO{
				ResolveDirectory: filesystem.ResolveMember,
				Rename:           filesystem.RenameNoReplace,
				PendingCovers:    repo.HasPendingRenameCovers,
				Relocate: func(context.Context, string, string, string, string) error {
					if conflict {
						if err := os.Mkdir(m.FolderPath, 0o755); err != nil {
							return err
						}
					}
					return errors.New("persistent sync failure")
				},
			})
			unit, _ := plannedUnit(t, task, m)
			prepared, err := task.PrepareUnit(t.Context(), workset.UnitRunInput{WorksetRoot: detailRoot(m), Unit: unit})
			if err != nil {
				t.Fatal(err)
			}
			result, err := prepared.Commit(t.Context())
			var facts dirrename.Result
			if decodeErr := json.Unmarshal(result.TaskResult, &facts); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			want := "RENAME_STATE_SYNC_FAILED"
			if conflict {
				want = "RENAME_ROLLBACK_FAILED"
			}
			if err != nil || result.ErrorCode != want || facts.Renamed != conflict || facts.RolledBack == conflict ||
				result.InventoryError == "" {
				t.Fatalf("result: %+v %+v %v", result, facts, err)
			}
			if conflict {
				if _, err := os.Stat(facts.TargetPath); err != nil {
					t.Fatal("lost renamed directory", err)
				}
			}
		})
	}
}
func TestRenameTargetRaceAndCancellationDoNotMoveSource(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "target claimed", true: "cancel"}[canceled], func(t *testing.T) {
			_, task, m := fixture(t, "title")
			unit, _ := plannedUnit(t, task, m)
			prepared, err := task.PrepareUnit(t.Context(), workset.UnitRunInput{WorksetRoot: detailRoot(m), Unit: unit})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if canceled {
				cancel()
			} else {
				if err = os.Mkdir(filepath.Join(detailRoot(m), "[RJ00000001] title"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			result, err := prepared.Commit(ctx)
			if canceled && (!errors.Is(err, context.Canceled) || !result.Canceled) ||
				!canceled && result.ErrorCode != "COMPONENT_RENAME_FAILED" {
				t.Fatalf("result: %+v %v", result, err)
			}
			if _, err = os.Stat(m.FolderPath); err != nil {
				t.Fatal("source moved", err)
			}
		})
	}
}
func TestRenameEmptyScopeAndNoopIgnorePendingCovers(t *testing.T) {
	repo, _, m := fixture(t, "title")
	calls := 0
	draft := []byte(`{"schema_version":1,"common":{"template":"{rj} old"},"members":[]}`)
	// Use the actual source spelling to produce a noop while preserving the canonical RJ.
	if err := os.Rename(m.FolderPath, filepath.Join(detailRoot(m), "RJ00000001 old")); err != nil {
		t.Fatal(err)
	}
	m.FolderPath = filepath.Join(detailRoot(m), "RJ00000001 old")
	m.RelPath = "RJ00000001 old"
	// Binding is deliberately frozen through a supplied catalog; no rescan is needed.
	task := dirrename.New(
		namedCatalog{map[string]dirrename.Work{m.FolderPath: {Code: "RJ00000001"}}},
		filesystem.ReadRenameDirectory,
		dirrename.ExecutionIO{
			ResolveDirectory: filesystem.ResolveMember,
			Rename:           filesystem.RenameNoReplace,
			Relocate:         repo.RelocateDirectoryAs,
			PendingCovers:    func(context.Context, string, []string) (bool, error) { calls++; return true, nil },
		},
	)
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: detailRoot(m), RawDraft: draft, Members: []*workset.WorksetMember{m}},
	)
	if err != nil {
		t.Fatal(err)
	}
	frozen, reasons, err := task.FreezeExecution(context.Background(), workset.RevisionFacts{Detail: detail(plan)})
	if err != nil || len(reasons) > 0 || frozen.TotalOperations != 0 || calls != 0 {
		t.Fatalf("noop: %+v %v %v", frozen, reasons, err)
	}
	_, err = task.NewExecutionSession(t.Context(), workset.ExecutionSessionInput{})
	if err != nil || calls != 0 {
		t.Fatalf("empty session: %v", err)
	}
	prepared, err := task.PrepareUnit(
		t.Context(),
		workset.UnitRunInput{WorksetRoot: detailRoot(m), Unit: frozen.Units[0]},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Commit(t.Context())
	if err != nil || result.ErrorCode != "" || *result.CompletedOperations != 0 {
		t.Fatalf("noop: %+v %v", result, err)
	}
}

func TestRenameSelectedInputsIgnoreMissingUnselectedRootAndKeepGlobalBlocks(t *testing.T) {
	_, task, m := fixture(t, "title")
	unit, d := plannedUnit(t, task, m)
	// Add an unselected root with a frozen valid proposal; its missing source
	// would stale the full plan, but neither selected execution entrance reads it.
	other := d.Roots[0]
	other.RootIndex = 1
	other.RootPath = filepath.Join(detailRoot(m), "gone")
	component := d.Components[0]
	component.ComponentIndex = 1
	component.RootIndex = 1
	component.ComponentID = "other"
	var o dirrename.Outcome
	if err := json.Unmarshal([]byte(component.OutcomeJSON), &o); err != nil {
		t.Fatal(err)
	}
	o.MemberID = "other"
	o.SourceName = "gone"
	raw, _ := json.Marshal(o)
	component.OutcomeJSON = string(raw)
	d.Roots = append(d.Roots, other)
	d.Components = append(d.Components, component)
	health, err := task.EvaluateRevision(
		context.Background(),
		workset.RevisionFacts{Detail: d, InputRoots: []string{m.FolderPath}},
	)
	if err != nil || health.InputMoved {
		t.Fatalf("selected freshness: %+v %v", health, err)
	}
	if _, err = task.NewExecutionSession(
		t.Context(),
		workset.ExecutionSessionInput{WorksetRoot: detailRoot(m), Units: []workset.ExecutionUnit{unit}, Detail: d},
	); err != nil {
		t.Fatal("worker read unselected root", err)
	}
	health, err = task.EvaluateRevision(context.Background(), workset.RevisionFacts{Detail: d})
	if err != nil || !health.InputMoved {
		t.Fatalf("whole plan missed missing input: %+v %v", health, err)
	}
	d.Components[1].Status = "blocked"
	health, err = task.EvaluateRevision(
		context.Background(),
		workset.RevisionFacts{Detail: d, InputRoots: []string{m.FolderPath}},
	)
	if err != nil || health.UnitsBlocked {
		t.Fatalf("a blocked folder is skipped, not a reason to refuse the run: %+v %v", health, err)
	}
}
