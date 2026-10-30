package workset_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestRevisionConclusionExcludedMembers(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("included", "excluded")
	f.insertAudioEntry("/music/included/track.mp3", "/music/included", 1024, 1000)
	ws := f.createCurrent("成员结论", ids...)
	excludedID := ws.Members[1].MemberID
	doc := draftDoc()
	doc.Members = []conversion.DraftMember{{MemberID: excludedID, Excluded: true}}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	gen := f.runGeneration(ws.WorksetID, saved.Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	detail, err := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(detail.Members))
	}
	for _, member := range detail.Members {
		conclusion := member
		if conclusion.MemberID != member.MemberID {
			t.Fatalf("member identity lost: %+v", conclusion)
		}
		if member.MemberID != excludedID {
			continue
		}
		if !conclusion.Excluded || conclusion.Input != "none" || conclusion.HasComponents {
			t.Fatalf("excluded member conclusion = %+v", conclusion)
		}
		var task struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(conclusion.Task, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status != "not_applicable" {
			t.Fatalf("excluded member task status = %q", task.Status)
		}
	}
}

type failedRevisionHealth struct{ workset.Task }

func (failedRevisionHealth) EvaluateRevision(context.Context, workset.RevisionFacts) (workset.RevisionHealth, error) {
	return workset.RevisionHealth{}, errors.New("inventory unavailable")
}

func TestRevisionConclusionRejectsFailedInputValidation(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("album")
	f.insertAudioEntry("/music/album/track.mp3", "/music/album", 1024, 1000)
	ws := f.createCurrent("输入校验", ids...)
	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	svc := workset.NewService(f.repo, 1, 1, []workset.Task{failedRevisionHealth{Task: &conversion.Task{}}}, nil, nil)
	if detail, err := svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID); err == nil {
		t.Fatalf("failed validation returned member input conclusions: %+v", detail.Members)
	}
}
