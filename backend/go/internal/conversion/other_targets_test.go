package conversion_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestNewDraftSeedsAllVersionTargetsAsMP3Only(t *testing.T) {
	task := conversion.New(nil, settings.NewReader(t.TempDir()), nil, nil, nil, nil)
	raw, _, _ := task.SeedDraft(nil)
	doc, err := conversion.ParseDraft(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Other == nil {
		t.Fatal("missing other version target")
	}
	for _, profile := range []reconcile.DesiredProfile{doc.Matched, doc.Unmatched, *doc.Other} {
		if profile.Lossless != nil || profile.Encoded == nil || profile.Encoded.Codec != reconcile.CodecMp3 ||
			profile.Encoded.Quality == nil || profile.Encoded.Quality.Bitrate != 320 {
			t.Fatalf("initial profile = %+v", profile)
		}
	}
}

func TestFreezeUsesTheOtherTargetOverrideFromTheRevision(t *testing.T) {
	wav := reconcile.DesiredProfile{Lossless: &reconcile.AudioOutputSpec{Codec: reconcile.CodecWav}}
	flac := reconcile.DesiredProfile{Lossless: &reconcile.AudioOutputSpec{Codec: reconcile.CodecFlac}}
	opus := reconcile.DesiredProfile{Encoded: &reconcile.AudioOutputSpec{
		Codec: reconcile.CodecOpus, Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 160},
	}}
	task := &conversion.Task{}
	members := []*workset.WorksetMember{{MemberID: "m", FolderPath: "work"}}
	for _, other := range []*reconcile.DesiredProfile{&opus, {}} {
		doc := &conversion.DraftDoc{
			SchemaVersion: 1, NoEffectTags: []string{"NoSE"}, Matched: wav, Unmatched: wav, Other: &flac,
			Members: []conversion.DraftMember{{MemberID: "m", Overrides: &conversion.OverrideSet{Other: other}}},
		}
		raw, _, marshalErr := conversion.MarshalDraft(doc)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		common := conversion.ProjectCommonSettings(doc)
		replaced := conversion.ReplaceCommonSettings(doc, common)
		effective, resolveErr := conversion.ResolveEffective(replaced, members)
		if resolveErr != nil || effective[0].Sources[conversion.UnitOther] != conversion.SourceMember {
			t.Fatalf("other inheritance = %+v, %v", effective, resolveErr)
		}
		frozen, blocked, freezeErr := task.FreezeExecution(context.Background(), workset.RevisionFacts{
			DraftSnapshot: []byte(raw), Members: members,
			Detail: &workset.PlanDetail{
				Roots: []workset.PlanRootRecord{{RootIndex: 0, RootPath: "work"}},
				Components: []workset.PlanComponentRecord{
					{RootIndex: 0, Partition: "unmatched", OutcomeJSON: `{}`},
					{RootIndex: 0, Partition: "matched", OutcomeJSON: `{"scope":"revert"}`},
					{RootIndex: 0, Partition: "unmatched", OutcomeJSON: `{"scope":"xxx"}`},
				},
			},
		})
		if freezeErr != nil || len(blocked) != 0 || len(frozen.Units) != 3 {
			t.Fatalf("freeze = %+v, %v, %v", frozen, blocked, freezeErr)
		}
		for i, unit := range frozen.Units {
			var got reconcile.DesiredProfile
			if decodeErr := json.Unmarshal(unit.Payload, &got); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			want := *other
			if i == 0 {
				want = wav
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("unit %d profile = %+v, want %+v", i, got, want)
			}
		}
	}
}
