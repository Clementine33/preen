package dirrename_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Clementine33/preen/backend/internal/dirrename"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestTitleFiltersUseNearestSameKindTerminatorAndKeepSpaces(t *testing.T) {
	for _, tc := range []struct {
		title, want             string
		brackets, tildes, books bool
	}{
		{"Title 【特典】 Vol.2", "Title  Vol.2", true, false, false},
		{"a【one】b【two】c", "abc", true, false, false},
		{"【a【b】Title", "Title", true, false, false},
		{"Title【open", "Title【open", true, false, false},
		{"Title~a~～b～〜c〜", "Title〜c〜", false, true, false},
		{"Title~mixed～", "Title~mixed～", false, true, false},
		{"Title〜mixed～", "Title〜mixed～", false, true, false},
		{"Title~a~b~c~", "Titleb", false, true, false},
		{"Title【a】~b~", "Title", true, true, false},
		{"Title【a】~b~《c》", "Title【a】~b~《c》", false, false, false},
		{"Title 《特典》 Vol.2", "Title  Vol.2", false, false, true},
		{"a《one》b《two》c", "abc", false, false, true},
		{"《a《b》Title", "Title", false, false, true},
		{"Title《open", "Title《open", false, false, true},
		{"Title【a】~b~《c》〜word〜", "Title〜word〜", true, true, true},
	} {
		t.Run(tc.title, func(t *testing.T) {
			source := "RJ000001 old"
			m := &workset.WorksetMember{MemberID: "m", RelPath: source, FolderPath: "/library/" + source}
			values := map[string]string{"title": tc.title}
			task := dirrename.New(
				namedCatalog{map[string]dirrename.Work{m.FolderPath: {Code: "RJ00000001", Values: values}}},
				func(context.Context, string) (dirrename.Directory, error) {
					return dirrename.Directory{source: {Directory: true}}, nil
				},
			)
			draft, _ := json.Marshal(
				dirrename.Draft{
					SchemaVersion: 1,
					Common: dirrename.Common{
						Template:            "[{rj}] {title}",
						StripBracketedTitle: tc.brackets,
						StripTildeTitle:     tc.tildes,
						StripBookTitle:      tc.books,
					},
					Members: []dirrename.Member{},
				},
			)
			plan, err := task.PlanSession(
				t.Context(),
				workset.PlanSessionInput{
					LibraryRoot: "/library",
					RawDraft:    draft,
					Members:     []*workset.WorksetMember{m},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			var o dirrename.Outcome
			if err = json.Unmarshal(plan.Units[0].Payload, &o); err != nil {
				t.Fatal(err)
			}
			if o.RawName != "[RJ00000001] "+tc.want {
				t.Fatalf("raw name %q", o.RawName)
			}
			if values["title"] != tc.title {
				t.Fatal("changed catalog metadata")
			}
			health, err := task.EvaluateRevision(context.Background(), workset.RevisionFacts{Detail: detail(plan)})
			if err != nil || health.InputMoved {
				t.Fatalf("freshness does not reuse filters: %+v %v", health, err)
			}
		})
	}
}

func TestDisabledFiltersKeepOldDraftHashAndEmptyFilteredTitleBlocks(t *testing.T) {
	_, task, m := fixture(t, "【only】")
	old, oldHash, _ := task.SeedDraft(nil)
	explicit := []byte(
		`{"schema_version":1,"common":{"template":"[{rj}] {title}","strip_bracketed_title":false,"strip_tilde_title":false,"strip_book_title":false},"members":[]}`,
	)
	normalized, hash, version, err := task.NormalizeDraft(explicit, []*workset.WorksetMember{m})
	if err != nil || string(normalized) != string(old) || hash != oldHash || version != 1 {
		t.Fatalf("legacy draft changed: %s %v", normalized, err)
	}
	raw := []byte(
		`{"schema_version":1,"common":{"template":"[{rj}] {title}","strip_bracketed_title":true},"members":[]}`,
	)
	plan, err := task.PlanSession(
		t.Context(),
		workset.PlanSessionInput{LibraryRoot: detailRoot(m), RawDraft: raw, Members: []*workset.WorksetMember{m}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Units[0].ReasonCode != "METADATA_MISSING" {
		t.Fatalf("accepted empty filtered title: %+v", plan.Units)
	}
}

func TestCommonSaveTurningFiltersOffReplacesTheWholeGroup(t *testing.T) {
	_, task, _ := fixture(t, "title")
	raw := []byte(
		`{"schema_version":1,"common":{"template":"[{rj}] {title}","strip_bracketed_title":true,"strip_tilde_title":true,"strip_book_title":true},"members":[]}`,
	)
	updated, _, err := task.ApplyCommonSettings(raw, json.RawMessage(`{"template":"[{rj}] {title}"}`))
	if err != nil {
		t.Fatal(err)
	}
	old, _, _ := task.SeedDraft(nil)
	if string(updated) != string(old) {
		t.Fatalf("omitted switches stayed enabled: %s", updated)
	}
}
