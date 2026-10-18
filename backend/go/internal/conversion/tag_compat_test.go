package conversion_test

import (
	"encoding/json"
	"strings"
	"testing"

	appconfig "github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestLegacyDraftTagsSurviveCommonAndRevisionReads(t *testing.T) {
	legacy := []byte(
		`{"schema_version":1,"classifier_tags":["SEなし"],"matched":{},"unmatched":{},"members":[{"member_id":"one","overrides":{"classifier_tags":[]}}]}`,
	)
	task := new(conversion.Task)
	doc, err := conversion.ParseDraft(string(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.NoEffectTags) != 1 || doc.Members[0].Overrides.NoEffectTags == nil ||
		len(*doc.Members[0].Overrides.NoEffectTags) != 0 {
		t.Fatalf("legacy explicit empty override lost: %+v", doc)
	}
	seedTask := conversion.New(nil, appconfig.NewReader(t.TempDir()), nil, nil, nil, nil)
	seed, _, schema := seedTask.SeedDraft(json.RawMessage(`{"classifier_tags":["SEなし"],"matched":{},"unmatched":{}}`))
	seeded, seedErr := conversion.ParseDraft(string(seed))
	if seedErr != nil || schema != 1 || len(seeded.NoEffectTags) != 1 ||
		strings.Contains(string(seed), "classifier_tags") {
		t.Fatalf("legacy common seed = %s, %v", seed, seedErr)
	}
	common, err := task.CommonSettings(legacy)
	if err != nil || strings.Contains(string(common), "classifier_tags") ||
		!strings.Contains(string(common), "no_effect_tags") {
		t.Fatalf("project common = %s, %v", common, err)
	}
	rewritten, _, err := task.ApplyCommonSettings(
		legacy,
		json.RawMessage(`{"classifier_tags":["new"],"matched":{},"unmatched":{}}`),
	)
	if err != nil || strings.Contains(string(rewritten), "classifier_tags") {
		t.Fatalf("rewrite = %s, %v", rewritten, err)
	}
	read, err := task.RevisionMembers(
		workset.RevisionFacts{
			DraftSnapshot: legacy,
			Members:       []*workset.WorksetMember{{MemberID: "one", FolderPath: "work"}},
		},
	)
	if err != nil || len(read) != 1 || read[0].Sources["no_effect_tags"] != "member" ||
		strings.Contains(string(read[0].Payload), "classifier_tags") {
		t.Fatalf("frozen read = %+v, %v", read, err)
	}
	if err := task.ValidateDraft(legacy, []*workset.WorksetMember{{MemberID: "one"}}); err != nil {
		t.Fatalf("incomplete draft must save: %v", err)
	}
	if err := task.ValidateSessionInput(legacy, []*workset.WorksetMember{{MemberID: "one"}}); err == nil {
		t.Fatal("empty participating no-effect override must fail generation")
	}
}

func TestNewEmptyTagKeysWinOverLegacyValues(t *testing.T) {
	raw := `{"schema_version":1,"no_effect_tags":[],"classifier_tags":["old"],"other_tags":[],"matched":{},"unmatched":{},"members":[{"member_id":"one","overrides":{"no_effect_tags":[],"classifier_tags":["old"],"other_tags":[]}}]}`
	doc, err := conversion.ParseDraft(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.NoEffectTags == nil || len(doc.NoEffectTags) != 0 || doc.Members[0].Overrides.NoEffectTags == nil ||
		len(*doc.Members[0].Overrides.NoEffectTags) != 0 {
		t.Fatalf("new explicit empty keys lost: %+v", doc)
	}
	encoded, _, err := conversion.MarshalDraft(doc)
	if err != nil || strings.Contains(encoded, "classifier_tags") {
		t.Fatalf("legacy key leaked: %s, %v", encoded, err)
	}
	task := new(conversion.Task)
	rewritten, _, err := task.ApplyCommonSettings(
		[]byte(raw),
		json.RawMessage(`{"no_effect_tags":[],"classifier_tags":["old"],"other_tags":[],"matched":{},"unmatched":{}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := conversion.ParseDraft(string(rewritten))
	if err != nil || len(normalized.NoEffectTags) != 0 {
		t.Fatalf("common legacy fallback overrode []: %+v, %v", normalized, err)
	}
}
