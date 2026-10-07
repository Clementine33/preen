package reconcile_test

import (
	"encoding/json"
	"testing"

	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
)

func TestOtherSubtreesAggregateIndependently(t *testing.T) {
	profile := reconcile.DesiredProfile{
		Lossless: &reconcile.AudioOutputSpec{Codec: reconcile.CodecWav},
		Encoded: &reconcile.AudioOutputSpec{
			Codec:   reconcile.CodecMp3,
			Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 320},
		},
	}
	policy := reconcile.Policy{
		SchemaVersion: 1,
		NoEffectTags:  []string{"se_off"},
		OtherTags:     []string{"revert", "xxx"},
		Matched:       profile,
		Unmatched:     profile,
	}
	classifier, err := reconcile.ResolveClassifier(policy.NoEffectTags)
	if err != nil {
		t.Fatal(err)
	}
	var entries []reconcile.AudioEntry
	for _, path := range []string{"wav/1.wav", "mp3/1.mp3", "se_off/1.wav", "se_off/mp3/1.mp3", "revert/1.wav", "revert/mp3/1.mp3", "wav/xxx/1.wav"} {
		entries = append(entries, reconcile.AudioEntry{PathPosix: "work/" + path, Bitrate: 320000})
	}
	for _, mode := range []string{reconcile.ModeStrict, reconcile.ModeAvailableSources} {
		t.Run(mode, func(t *testing.T) {
			policy.Mode = mode
			out, err := reconcile.Reconcile(
				reconcile.ReconcileInput{RootPath: "work", Entries: entries, Policy: policy, Classifier: classifier},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Components) != 4 || out.Summary.BlockedCount != 0 {
				t.Fatalf("want four independent unblocked components, got %+v", out)
			}
			counts := map[string]int{}
			for _, component := range out.Components {
				counts[component.Scope+":"+string(component.Partition)] = len(component.Files)
			}
			for key, count := range map[string]int{":matched": 2, ":unmatched": 2, "revert:unmatched": 2, "wav/xxx:unmatched": 1} {
				if counts[key] != count {
					t.Fatalf("%s files = %d, want %d", key, counts[key], count)
				}
			}
		})
	}
}

func TestOtherScopesRespectDirectoryBoundaries(t *testing.T) {
	profile := reconcile.DesiredProfile{Lossless: &reconcile.AudioOutputSpec{Codec: reconcile.CodecWav}}
	for _, tc := range []struct {
		name        string
		paths, tags []string
		want        map[string]int
	}{
		{"flat format variants", []string{"WAV/1.wav", "MP3/1.mp3", "WAV_NoSE/1.wav", "MP3_NoSE/1.mp3"}, nil, map[string]int{":matched": 2, ":unmatched": 2}},
		{"outermost root and original path", []string{"se_off/revert/1.wav", "se_off/revert/xxx/1.mp3"}, []string{"xxx", "REVERT"}, map[string]int{"se_off/revert:matched": 2}},
		{"different actual roots stay separate", []string{"revert/1.wav", "revert_copy/1.wav", "wav/xxx/1.wav"}, []string{"revert", "xxx"}, map[string]int{"revert:unmatched": 1, "revert_copy:unmatched": 1, "wav/xxx:unmatched": 1}},
		{"filenames do not isolate", []string{"wav/xxx.wav", "mp3/xxx.mp3"}, []string{"xxx"}, map[string]int{":unmatched": 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := reconcile.Policy{
				SchemaVersion: 1,
				NoEffectTags:  []string{"NoSE", "se_off"},
				OtherTags:     tc.tags,
				Matched:       profile,
				Unmatched:     profile,
			}
			classifier, err := reconcile.ResolveClassifier(policy.NoEffectTags)
			if err != nil {
				t.Fatal(err)
			}
			entries := make([]reconcile.AudioEntry, 0, len(tc.paths))
			for _, path := range tc.paths {
				entries = append(entries, reconcile.AudioEntry{PathPosix: "work/" + path, Bitrate: 320000})
			}
			out, err := reconcile.Reconcile(
				reconcile.ReconcileInput{RootPath: "work", Entries: entries, Policy: policy, Classifier: classifier},
			)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]int{}
			for _, c := range out.Components {
				got[c.Scope+":"+string(c.Partition)] += len(c.Files)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("scopes = %+v, want %+v", got, tc.want)
			}
			for key, count := range tc.want {
				if got[key] != count {
					t.Fatalf("scopes = %+v, want %+v", got, tc.want)
				}
			}
		})
	}
}

func TestRegisteringOtherTagResolvesRealSourceConflict(t *testing.T) {
	profile := reconcile.DesiredProfile{
		Encoded: &reconcile.AudioOutputSpec{
			Codec:   reconcile.CodecMp3,
			Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 320},
		},
	}
	classifier, err := reconcile.ResolveClassifier([]string{"NoSE"})
	if err != nil {
		t.Fatal(err)
	}
	entries := []reconcile.AudioEntry{{PathPosix: "work/wav/1.wav"}, {PathPosix: "work/wav/xxx/1.wav"}}
	for _, mode := range []string{reconcile.ModeStrict, reconcile.ModeAvailableSources} {
		t.Run(mode, func(t *testing.T) {
			policy := reconcile.Policy{
				SchemaVersion: 1,
				Mode:          mode,
				NoEffectTags:  []string{"NoSE"},
				Matched:       profile,
				Unmatched:     profile,
			}
			input := reconcile.ReconcileInput{
				RootPath:   "work",
				Entries:    entries,
				Policy:     policy,
				Classifier: classifier,
			}
			before, err := reconcile.Reconcile(input)
			if err != nil {
				t.Fatal(err)
			}
			if before.Summary.BlockedCount != 1 || before.Components[0].ReasonCode != reconcile.ReasonSourceAmbiguous {
				t.Fatalf("expected source conflict: %+v", before)
			}
			paths := map[string]bool{}
			for _, v := range before.Components[0].Variants {
				for _, d := range v.Decisions {
					if d.ReasonCode == reconcile.ReasonSourceAmbiguous {
						paths[d.Path] = true
					}
				}
			}
			if len(paths) != 2 || !paths[entries[0].PathPosix] || !paths[entries[1].PathPosix] {
				t.Fatalf("missing competing source paths: %+v", paths)
			}
			input.Policy.OtherTags = []string{"xxx"}
			after, err := reconcile.Reconcile(input)
			if err != nil {
				t.Fatal(err)
			}
			if after.Summary.BlockedCount != 0 || len(after.Components) != 2 {
				t.Fatalf("expected independent scopes: %+v", after)
			}
			input.Policy.OtherTags = []string{"unused"}
			unchanged, err := reconcile.Reconcile(input)
			if err != nil {
				t.Fatal(err)
			}
			if unchanged.Components[0].ComponentID != before.Components[0].ComponentID {
				t.Fatal("unused tags changed component identity")
			}
		})
	}
}

func TestPersistedLegacyPolicyStillReconciles(t *testing.T) {
	var policy reconcile.Policy
	err := json.Unmarshal(
		[]byte(
			`{"schema_version":1,"classifier_tags":["NoSE"],"matched":{"lossless":{"codec":"wav"}},"unmatched":{"lossless":{"codec":"wav"}}}`,
		),
		&policy,
	)
	if err != nil {
		t.Fatal(err)
	}
	if validateErr := reconcile.ValidatePolicy(policy); validateErr != nil {
		t.Fatalf("legacy validation: %v", validateErr)
	}
	classifier, err := reconcile.ResolveClassifier([]string{"NoSE"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := reconcile.Reconcile(
		reconcile.ReconcileInput{
			RootPath:   "work",
			Entries:    []reconcile.AudioEntry{{PathPosix: "work/1.wav"}},
			Policy:     policy,
			Classifier: classifier,
		},
	)
	if err != nil || out.Summary.BlockedCount != 0 || len(out.Components) != 1 {
		t.Fatalf("legacy reconciliation: %+v, %v", out, err)
	}
}
