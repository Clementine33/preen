package works_test

import (
	"slices"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works"
)

var (
	chain = []works.LanguageEdition{
		{VersionRootRJ: "RJ01289610", Language: "CHI_HANS", Label: new("简中")},
		{VersionRootRJ: "RJ01291590", Language: "CHI_HANT", Label: new("繁中")},
		{VersionRootRJ: "RJ01299999", Language: "ENG"},
	}
	chainChildren = map[works.WorkCode][]works.WorkCode{
		"RJ01289610": {"RJ01289700", "RJ01289611"},
	}
	chainPresence = map[works.WorkCode]works.Presence{
		"RJ01287230": {IsWork: true, Held: true},
		"RJ01289610": {IsWork: true},
		"RJ01289611": {IsWork: true, Held: true},
		"RJ01289700": {IsWork: true},
	}
)

func rjs(slots []works.EditionSlot) []string {
	var out []string
	for _, s := range slots {
		out = append(out, s.Language+":"+string(s.Root.RJ))
		for _, n := range s.Translations {
			out = append(out, string(n.RJ))
		}
	}
	return out
}

func TestEditionTreeFromTheOriginalsPage(t *testing.T) {
	got := works.BuildEditionTree("RJ01287230", chain, works.ResolvedRelation{Role: new(works.RoleOriginal)},
		chainChildren, chainPresence)
	want := []string{"JPN:RJ01287230", "CHI_HANS:RJ01289610", "RJ01289611", "RJ01289700", "CHI_HANT:RJ01291590"}
	if !slices.Equal(rjs(got), want) {
		t.Fatalf("tree = %v, want %v (other languages dropped, translations ordered by RJ)", rjs(got), want)
	}
	if !got[0].Held || !got[1].Held || got[2].Held {
		t.Errorf("held = %v %v %v, want JPN and the held-translation slot only", got[0].Held, got[1].Held, got[2].Held)
	}
	if got[1].Root.IsWork != true || got[1].Root.Held || got[2].Root.IsWork {
		t.Errorf("edition page flags wrong: %+v / %+v", got[1].Root, got[2].Root)
	}
}

func TestEditionTreeFromAnEditionPageAndATranslation(t *testing.T) {
	zh := "CHI_HANS"
	orig := works.WorkCode("RJ01287230")
	page := works.BuildEditionTree(
		"RJ01289610",
		chain,
		works.ResolvedRelation{
			Role:       new(works.RoleParent),
			Language:   &zh,
			OriginalID: &orig,
		},
		chainChildren,
		chainPresence,
	)
	if got := rjs(page); got[0] != "JPN:RJ01287230" || got[1] != "CHI_HANS:RJ01289610" {
		t.Errorf("page tree = %v", got)
	}

	parent := works.WorkCode("RJ01289610")
	child := works.BuildEditionTree("RJ01289611", chain,
		works.ResolvedRelation{Role: new(works.RoleChild), Language: &zh, OriginalID: &orig, ParentID: &parent},
		chainChildren, chainPresence)
	if child[1].Translations[0].RJ != "RJ01289611" || child[1].Translations[0].Role != works.TreeRoleTranslation {
		t.Errorf("translation not listed under its page: %+v", child[1])
	}
}

// A held 繁中 translation shows under a page nobody fetched: the page is known
// only through the translation's parent, and the work list has no entry.
func TestEditionTreeKnowsAPageOnlyThroughItsTranslation(t *testing.T) {
	zh := "CHI_HANT"
	orig, parent := works.WorkCode("RJ01287230"), works.WorkCode("RJ01291590")
	got := works.BuildEditionTree("RJ01291591", nil,
		works.ResolvedRelation{Role: new(works.RoleChild), Language: &zh, OriginalID: &orig, ParentID: &parent},
		nil, map[works.WorkCode]works.Presence{"RJ01291591": {IsWork: true, Held: true}})
	want := []string{"JPN:RJ01287230", "CHI_HANT:RJ01291590", "RJ01291591"}
	if !slices.Equal(rjs(got), want) {
		t.Fatalf("tree = %v, want %v", rjs(got), want)
	}
	if got[1].Root.IsWork || !got[1].Held {
		t.Errorf("page = %+v held=%v, want a non-work page held through its translation", got[1].Root, got[1].Held)
	}
}

func TestEditionTreeIsNilWithoutAnyOtherNode(t *testing.T) {
	nm := works.BuildEditionTree("RJ00000001", nil, works.ResolvedRelation{Role: new(works.RoleOriginal)}, nil, nil)
	onlyOther := works.BuildEditionTree(
		"RJ00000001",
		[]works.LanguageEdition{{VersionRootRJ: "RJ00000009", Language: "ENG"}},
		works.ResolvedRelation{Role: new(works.RoleOriginal)},
		nil,
		nil,
	)
	if nm != nil || onlyOther != nil {
		t.Errorf("tree = %v / %v, want nil", nm, onlyOther)
	}
}
