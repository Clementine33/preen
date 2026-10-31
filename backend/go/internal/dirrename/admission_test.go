package dirrename_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func metadataRecord(t *testing.T, f renameRun, otherLibrary bool) string {
	t.Helper()
	if !otherLibrary {
		return f.worksetID
	}
	libraryID := f.libraryID
	if otherLibrary {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, f.member.RelPath), 0o755); err != nil {
			t.Fatal(err)
		}
		lib, err := f.repo.CreateLibrary("other library", root)
		if err != nil {
			t.Fatal(err)
		}
		pipeline := inventory.NewPipeline(
			sqlite.NewScanStaging(f.repo),
			filesystem.WalkRootEntriesParallel,
			filesystem.WalkFolderEntries,
			nil,
		)
		if _, err := pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(lib.ID)); err != nil {
			t.Fatal(err)
		}
		libraryID = lib.ID
	}
	created, err := f.svc.CreateCurrentWorkset(
		t.Context(),
		workset.CreateCurrentRequest{
			LibraryID:     libraryID,
			OperationType: "metadata_refresh",
			Selection:     workset.Selection{All: true},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return created.Workset.WorksetID
}
func TestRenameAndSharedBatchGenerationExcludeEachOther(t *testing.T) {
	for _, renameFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "generation first", true: "rename first"}[renameFirst], func(t *testing.T) {
			f := newRenameRun(t)
			other := metadataRecord(t, f, false)
			if renameFirst {
				f.start(t)
				_, err := f.svc.StartGeneration(
					t.Context(),
					other,
					"metadata_refresh",
					workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "other"},
				)
				e, ok := workset.AsError(err)
				if !ok || e.Code != "EXECUTION_IN_PROGRESS" {
					t.Fatalf("generation: %v", err)
				}
			} else {
				if _, err := f.svc.StartGeneration(
					t.Context(),
					other,
					"metadata_refresh",
					workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "other"},
				); err != nil {
					t.Fatal(err)
				}
				op, err := f.svc.GetOperation(t.Context(), f.worksetID, "rename")
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.svc.StartExecution(
					t.Context(),
					f.worksetID,
					"rename",
					f.planID,
					workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "execute"},
				)
				e, ok := workset.AsError(err)
				if !ok || e.Code != "PLAN_NOT_EXECUTABLE" {
					t.Fatalf("rename: %v", err)
				}
			}
		})
	}
}
func TestOtherLibraryGenerationOnlyBlocksRenameDuringRegisteredScan(t *testing.T) {
	f := newRenameRun(t)
	other := metadataRecord(t, f, true)
	if _, err := f.svc.StartGeneration(
		t.Context(),
		other,
		"metadata_refresh",
		workset.StartGenerationRequest{IfMatchVersion: 1, IdempotencyKey: "other"},
	); err != nil {
		t.Fatal(err)
	}
	// Generation's inventory refresh uses this same global count. Its queued
	// session in another library alone does not block rename.
	release, err := f.gate.BeginScan()
	if err != nil {
		t.Fatal(err)
	}
	op, err := f.svc.GetOperation(t.Context(), f.worksetID, "rename")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.StartExecution(
		t.Context(),
		f.worksetID,
		"rename",
		f.planID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "execute"},
	)
	if !admission.IsBusy(err) {
		t.Fatalf("registered other-library refresh did not block: %v", err)
	}
	release()
	f.start(t)
}
func TestMemberAndLibraryRefreshExcludeQueuedAndRunningRename(t *testing.T) {
	for _, status := range []string{"queued", "running"} {
		t.Run(status, func(t *testing.T) {
			f := newRenameRun(t)
			service := inventory.NewService(nil, f.repo, f.gate, nil)
			release, err := service.AdmitMemberRefresh(detailRoot(f.member))
			if err != nil {
				t.Fatal(err)
			}
			op, err := f.svc.GetOperation(t.Context(), f.worksetID, "rename")
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.svc.StartExecution(
				t.Context(),
				f.worksetID,
				"rename",
				f.planID,
				workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "execute"},
			)
			if !admission.IsBusy(err) {
				t.Fatalf("member refresh did not exclude rename: %v", err)
			}
			release()
			ex := f.start(t)
			if _, err = f.repo.DB().
				Exec(`UPDATE plan_executions SET status=? WHERE execution_id=?`, status, ex.ExecutionID); err != nil {
				t.Fatal(err)
			}
			for _, admit := range []func(string) (func(), error){service.AdmitMemberRefresh, service.AdmitLibraryScan} {
				if _, err = admit(detailRoot(f.member)); err == nil {
					t.Fatal("scan admitted during rename")
				}
			}
			other, err := service.AdmitMemberRefresh(t.TempDir())
			if err != nil {
				t.Fatal("other library refused", err)
			}
			other()
		})
	}
}
func TestPendingCoverAdmissionUsesFrozenPathsAndSelectedScope(t *testing.T) {
	f := newRenameRun(t)
	ex := f.start(t)
	targets, err := f.repo.CoverTargets(t.Context(), detailRoot(f.member), []string{f.member.FolderPath})
	if err != nil {
		t.Fatal(err)
	}
	target := targets[f.member.FolderPath]
	raw, _ := json.Marshal(target)
	if _, err = f.repo.DB().
		Exec(`INSERT INTO cover_deliveries(id,execution_id,work_code,locale,copy_id,target_json,refresh,status) VALUES('delivery',?,'RJ00000001','ja-jp',?,?,0,'running')`, ex.ExecutionID, target.CopyID, string(raw)); err != nil {
		t.Fatal(err)
	}
	d, err := f.repo.GetPlanDetail(f.planID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = f.task.FreezeExecution(context.Background(), workset.RevisionFacts{Detail: d})
	e, ok := workset.AsError(err)
	if !ok || e.Code != "COVER_DELIVERY_PENDING" {
		t.Fatalf("cover admission: %v", err)
	}
	if _, _, err = f.task.FreezeExecution(
		context.Background(),
		workset.RevisionFacts{Detail: d, InputRoots: []string{}},
	); err != nil {
		t.Fatal("empty scope checked covers", err)
	}
	if _, err = f.repo.DB().
		Exec(`UPDATE cover_deliveries SET status='failed',error_code='INVENTORY_REFRESH_FAILED'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.task.FreezeExecution(context.Background(), workset.RevisionFacts{Detail: d}); err != nil {
		t.Fatal("failed delivery blocked rename", err)
	}
}
