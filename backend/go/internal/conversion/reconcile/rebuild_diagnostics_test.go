package reconcile_test

import (
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

func TestStrictRebuildDoesNotCallSatisfiedTargetUnconfirmed(t *testing.T) {
	classifier, err := reconcile.ResolveClassifier([]string{"NoSE"})
	if err != nil {
		t.Fatal(err)
	}
	profile := reconcile.DesiredProfile{Encoded: &reconcile.AudioOutputSpec{
		Codec:   reconcile.CodecMp3,
		Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 320},
	}}
	result, err := reconcile.Reconcile(reconcile.ReconcileInput{
		RootPath:   "/work",
		Classifier: classifier,
		Policy: reconcile.Policy{
			SchemaVersion: 1,
			NoEffectTags:  []string{"NoSE"},
			Matched:       profile,
			Unmatched:     profile,
		},
		Entries: []reconcile.AudioEntry{
			{PathPosix: "/work/wav/00.wav"},
			{PathPosix: "/work/mp3/00.mp3", Bitrate: 320000},
			{PathPosix: "/work/wav/01.wav"},
			{PathPosix: "/work/mp3/01　.mp3", Bitrate: 320000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Components) != 1 {
		t.Fatalf("components: %d, want 1", len(result.Components))
	}
	component := result.Components[0]
	if component.Status != reconcile.StatusBlocked || len(component.Operations) != 0 {
		t.Fatalf("strict rebuild must remain blocked without a source: %+v", component)
	}
	if component.ReasonCode != reconcile.ReasonSourceMissing ||
		!strings.Contains(component.Message, "component-wide") ||
		strings.Contains(component.Message, "unconfirmed") {
		t.Fatalf(
			"satisfied target must report missing rebuild source, got %s: %s",
			component.ReasonCode,
			component.Message,
		)
	}
	for _, variant := range component.Variants {
		for _, decision := range variant.Decisions {
			if strings.HasSuffix(decision.Path, ".mp3") && decision.ReasonCode != reconcile.ReasonKeepEncodedSatisfied {
				t.Fatalf("satisfied MP3 must retain its own conformance fact: %+v", decision)
			}
		}
	}
}
