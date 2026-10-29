package workset_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestMemberReviewReadsOnlyOwnedFrozenPlan(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA", "albumB", "excluded", "empty")
	f.insertAudioEntry("/music/albumA/own.mp3", "/music/albumA", 1024, 1000)
	f.insertAudioEntry("/music/albumB/other.mp3", "/music/albumB", 1024, 1000)
	ws := f.createCurrent("成员审阅", ids...)
	byPath := map[string]string{}
	for _, m := range ws.Members {
		byPath[m.RelPath] = m.MemberID
	}
	tags := []string{"frozen"}
	doc := draftDoc()
	doc.Members = []conversion.DraftMember{
		{MemberID: byPath["albumA"], Overrides: &conversion.OverrideSet{NoEffectTags: &tags}},
		{MemberID: byPath["excluded"], Excluded: true},
	}
	saved := f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	gen := f.runGeneration(ws.WorksetID, saved.Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	review, err := f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, byPath["albumA"])
	if err != nil {
		t.Fatal(err)
	}
	if review.Member.MemberID != byPath["albumA"] || review.Member.Sources["no_effect_tags"] != "member" ||
		review.Member.Sources["mode"] != "common" {
		t.Fatalf("frozen member = %+v", review.Member)
	}
	if !strings.Contains(string(review.Member.Payload), "frozen") || len(review.Roots) != 1 ||
		review.Roots[0].RootPath != "/music/albumA" ||
		len(review.Plan.Units) != 1 {
		t.Fatalf("member review = %+v", review)
	}
	raw, _ := json.Marshal(review.Plan.Units)
	if strings.Contains(string(raw), "other.mp3") {
		t.Fatalf("other member decisions leaked: %s", raw)
	}
	if review.Member.Conclusion.Input != "valid" || review.Roots[0].Stale {
		t.Fatalf("fresh member input = %+v", review.Member.Conclusion)
	}
	f.insertAudioEntry("/music/albumA/new.mp3", "/music/albumA", 1024, 1000)
	stale, err := f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, byPath["albumA"])
	if err != nil {
		t.Fatal(err)
	}
	if stale.Member.Conclusion.Input != "stale" || !stale.Roots[0].Stale || len(stale.Plan.Units) != 1 {
		t.Fatalf("changed input must preserve the frozen component: %+v", stale)
	}
	for _, name := range []string{"excluded", "empty"} {
		empty, readErr := f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, byPath[name])
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(empty.Plan.Units) != 0 || empty.Member.Conclusion.HasComponents {
			t.Fatalf("%s review = %+v", name, empty)
		}
		if name == "excluded" && (!empty.Member.Conclusion.Excluded || empty.Member.Conclusion.Input != "none") {
			t.Fatalf("excluded = %+v", empty.Member.Conclusion)
		}
	}
	_, err = f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, "foreign-member")
	assertMemberReviewError(t, err, "MEMBER_NOT_FOUND")
	doc.Members = nil
	f.saveDraft(ws.WorksetID, doc, f.operation(ws.WorksetID).Version)
	again, err := f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, byPath["albumA"])
	if err != nil || !strings.Contains(string(again.Member.Payload), "frozen") {
		t.Fatalf("frozen review changed: %+v, %v", again, err)
	}
	f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	_, err = f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, byPath["albumA"])
	assertMemberReviewError(t, err, "REVISION_NOT_FOUND")
}

func assertMemberReviewError(t *testing.T, err error, code string) {
	t.Helper()
	if e, ok := workset.AsError(err); !ok || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}
