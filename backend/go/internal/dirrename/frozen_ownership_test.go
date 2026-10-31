package dirrename_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestRenamePreservesFrozenMemberOwnershipAcrossOperations(t *testing.T) {
	repo, rename, original := fixture(t, "title")
	svc := workset.NewService(repo, 1, 1, []workset.Task{
		rename, refresh.NewTask(repo, filesystem.CheckWorkDirectory),
	}, nil, nil)
	libs, err := repo.ListLibraries()
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateCurrentWorkset(t.Context(), workset.CreateCurrentRequest{
		LibraryID: libs[0].ID, OperationType: "rename", Selection: workset.Selection{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, memberID := created.Workset.WorksetID, created.Workset.Members[0].MemberID
	svc.DispatcherHandle().Start()
	t.Cleanup(func() {
		if stopErr := svc.DispatcherHandle().Stop(context.Background()); stopErr != nil {
			t.Error(stopErr)
		}
	})
	plans := map[string]string{}
	frozen := map[string]json.RawMessage{}
	for _, kind := range []string{"metadata_refresh", "rename"} {
		plans[kind] = generateOwnershipPlan(t, svc, id, kind)
		revision, readErr := svc.GetRevision(t.Context(), id, kind, plans[kind])
		if readErr != nil {
			t.Fatal(readErr)
		}
		frozen[kind] = revision.Members[0].Task
	}
	// The existing HTTP seam starts a real rename, including filesystem commit
	// and atomic relocation of the shared membership used by both operations.
	verifyHTTPReviewAndExecution(t, svc, id, plans["rename"])
	waitOwnershipRename(t, svc, id)
	current, err := svc.GetWorkset(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if current.Members[0].MemberID != memberID || current.Members[0].FolderPath == original.FolderPath {
		t.Fatalf("rename did not preserve and relocate shared membership: %+v", current.Members)
	}
	for _, kind := range []string{"rename", "metadata_refresh"} {
		t.Run(kind, func(t *testing.T) {
			revision, err := svc.GetRevision(t.Context(), id, kind, plans[kind])
			if err != nil {
				t.Fatal(err)
			}
			if len(revision.Members) != 1 {
				t.Fatalf("members = %+v", revision.Members)
			}
			conclusion := revision.Members[0]
			if conclusion.MemberID != memberID || conclusion.Input != "stale" || !conclusion.HasComponents ||
				!bytes.Equal(conclusion.Task, frozen[kind]) {
				t.Fatalf("frozen conclusion lost after rename: %+v", conclusion)
			}
			review, err := svc.GetMemberReview(t.Context(), id, kind, plans[kind], memberID)
			if err != nil {
				t.Fatal(err)
			}
			if review.Member.Conclusion.Input != "stale" || !review.Member.Conclusion.HasComponents ||
				len(review.Roots) != 1 ||
				review.Roots[0].RootPath != original.FolderPath ||
				!review.Roots[0].Stale ||
				len(review.Plan.Units) != 1 ||
				!bytes.Equal(review.Plan.Units[0], frozen[kind]) ||
				!bytes.Equal(review.Member.Conclusion.Task, frozen[kind]) {
				t.Fatalf("frozen review lost after rename: %+v", review)
			}
		})
	}
}

func generateOwnershipPlan(t *testing.T, svc workset.Service, id, kind string) string {
	t.Helper()
	op, err := svc.GetOperation(t.Context(), id, kind)
	if err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartGeneration(t.Context(), id, kind, workset.StartGenerationRequest{
		IfMatchVersion: op.Version, IdempotencyKey: kind,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		generation, err := svc.GetGeneration(t.Context(), id, kind, started.Generation.GenerationID)
		if err != nil {
			t.Fatal(err)
		}
		if generation.Status == workset.GenStatusCompleted {
			return generation.RevisionID
		}
		if generation.Status == workset.GenStatusFailed {
			t.Fatalf("generation failed: %+v", generation)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("generation timeout")
	return ""
}

func waitOwnershipRename(t *testing.T, svc workset.Service, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		op, err := svc.GetOperation(t.Context(), id, "rename")
		if err != nil {
			t.Fatal(err)
		}
		if op.ActiveExecution == nil && op.LatestExecution != nil {
			if op.LatestExecution.Status != workset.ExecStatusSucceeded {
				t.Fatalf("rename failed: %+v", op.LatestExecution)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("rename timeout")
}
