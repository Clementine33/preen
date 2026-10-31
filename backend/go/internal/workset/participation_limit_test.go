package workset_test

import (
	"testing"

	appconfig "github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// limitedConversion lowers conversion's declared limit so the cap is reachable
// inside the batch limit.
type limitedConversion struct{ *conversion.Task }

func (limitedConversion) ParticipationLimit() int { return 2 }

func TestParticipationLimitRefusesAndCountsOnlyParticipants(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("a", "b", "c")
	ws := f.createCurrent("上限", ids...)
	base := conversion.New(f.repo, appconfig.NewReader(f.configDir), nil, nil, nil, nil, conversion.ExecutionIO{})
	workset.RegisterTasksForTest(f.svc, []workset.Task{limitedConversion{base}})

	if got := f.operation(ws.WorksetID).MaxParticipants; got != 2 {
		t.Fatalf("MaxParticipants = %d, want 2", got)
	}
	version := f.operation(ws.WorksetID).Version
	_, err := f.svc.StartGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion,
		workset.StartGenerationRequest{IfMatchVersion: version, IdempotencyKey: "k1"})
	werr, ok := workset.AsError(err)
	if !ok || werr.Code != "TOO_MANY_PARTICIPANTS" || werr.Kind != workset.ErrKindInvalidArgument ||
		len(werr.Details) != 2 || werr.Details[0] != "limit=2" || werr.Details[1] != "participants=3" {
		t.Fatalf("err = %+v", err)
	}

	doc := draftDoc()
	doc.Members = []conversion.DraftMember{{MemberID: ws.Members[2].MemberID, Excluded: true}}
	saved := f.saveDraft(ws.WorksetID, doc, version)
	if _, err := f.svc.StartGeneration(f.ctx, ws.WorksetID, workset.OperationTypeConversion,
		workset.StartGenerationRequest{IfMatchVersion: saved.Version, IdempotencyKey: "k2"}); err != nil {
		t.Fatalf("excluded member must not count: %v", err)
	}
}

func TestParticipationLimitIsAbsentForTasksWithoutOne(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("a")
	ws := f.createCurrent("无上限", ids...)
	workset.RegisterTasksForTest(f.svc, []workset.Task{stubTask{kind: "conversion", seed: "{}"}})
	if got := f.operation(ws.WorksetID).MaxParticipants; got != 0 {
		t.Fatalf("MaxParticipants = %d, want 0", got)
	}
}
