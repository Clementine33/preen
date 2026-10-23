package sqlite //nolint:testpackage // white-box tests seed the catalog through the real repository

import (
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

const testStamp = "2026-09-27T10:00:00.000000000Z"

func code(t *testing.T, raw string) works.WorkCode {
	t.Helper()
	parsed, ok := works.ParseWorkCode(raw)
	if !ok {
		t.Fatalf("test uses an invalid work number: %s", raw)
	}
	return parsed
}

// seedWork inserts a collected catalog fixture.
func seedWork(t *testing.T, repo *Repository, raw string) works.WorkCode {
	t.Helper()
	id := code(t, raw)
	if _, err := repo.DB().Exec(
		"INSERT INTO works (id, title, collected_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		string(id), "title of "+raw, testStamp, testStamp, testStamp,
	); err != nil {
		t.Fatalf("seed %s: %v", raw, err)
	}
	return id
}

func coreGroup(t *testing.T, source works.Source, title string) *works.CoreGroup {
	t.Helper()
	return &works.CoreGroup{Source: source, FetchedAt: mustTime(t, testStamp), Title: &title}
}

func relationsGroup(t *testing.T, role works.RelationRole, parent *string, children ...string) *works.RelationsGroup {
	t.Helper()
	group := &works.RelationsGroup{
		Source:    works.SourceDLsite,
		FetchedAt: mustTime(t, testStamp),
		Role:      role,
	}
	if parent != nil {
		parsed := code(t, *parent)
		group.ParentRJ = &parsed
	}
	for _, child := range children {
		group.ChildRJs = append(group.ChildRJs, code(t, child))
	}
	return group
}

func mustTime(t *testing.T, raw string) time.Time {
	t.Helper()
	at, err := time.Parse(works.TimestampFormat, raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return at
}

func importGroups(
	t *testing.T,
	repo *Repository,
	target works.WorkCode,
	in *works.ValidatedImport,
) works.ImportOutcome {
	t.Helper()
	outcome, err := repo.ImportMetadata(in, mustTime(t, testStamp))
	if err != nil {
		t.Fatalf("import into %s: %v", target, err)
	}
	return outcome
}

func TestImportAppliesGroupsAndDerivesRelations(t *testing.T) {
	repo := newTestRepository(t)
	parent := seedWork(t, repo, "RJ00000001")
	child := seedWork(t, repo, "RJ00000002")
	grandchild := seedWork(t, repo, "RJ00000003")

	// The parent says what it is; the child declares the parent without
	// reporting anything itself, so it inherits; the grandchild is declared by
	// the child.
	language := "JPN"
	importGroups(t, repo, parent, &works.ValidatedImport{
		Target:     parent,
		Core:       coreGroup(t, works.SourceDLsite, "the original"),
		Statistics: &works.StatisticsGroup{FetchedAt: mustTime(t, testStamp), DLCount: new(42)},
		Relations: &works.RelationsGroup{
			Source:    works.SourceDLsite,
			FetchedAt: mustTime(t, testStamp),
			Role:      works.RoleOriginal,
			Language:  &language,
			ChildRJs:  []works.WorkCode{child},
		},
	})
	importGroups(t, repo, child, &works.ValidatedImport{
		Target: child,
		Relations: &works.RelationsGroup{
			Source:    works.SourceDLsite,
			FetchedAt: mustTime(t, testStamp),
			Role:      works.RoleChild,
			ParentRJ:  &parent,
		},
	})
	importGroups(t, repo, grandchild, &works.ValidatedImport{
		Target: grandchild,
		Relations: &works.RelationsGroup{
			Source:    works.SourceDLsite,
			FetchedAt: mustTime(t, testStamp),
			Role:      works.RoleChild,
			ParentRJ:  &child,
		},
	})

	detail, found, err := repo.GetWork(parent)
	if err != nil || !found {
		t.Fatalf("GetWork(parent) = %v, %v", found, err)
	}
	if detail.Title == nil || *detail.Title != "the original" {
		t.Errorf("title = %v", detail.Title)
	}
	if detail.DLCount == nil || *detail.DLCount != 42 {
		t.Errorf("dl_count = %v, want 42", detail.DLCount)
	}
	if detail.Relation.Role == nil || *detail.Relation.Role != works.RoleOriginal {
		t.Errorf("role = %v, want original", detail.Relation.Role)
	}

	// The child never reported; it inherits the parent's language and original.
	childDetail, _, err := repo.GetWork(child)
	if err != nil {
		t.Fatal(err)
	}
	if childDetail.Relation.ParentID == nil || *childDetail.Relation.ParentID != parent {
		t.Errorf("child parent = %v, want %s", childDetail.Relation.ParentID, parent)
	}
	if childDetail.Relation.Language == nil || *childDetail.Relation.Language != "JPN" {
		t.Errorf("child language = %v, want the inherited JPN", childDetail.Relation.Language)
	}
	if childDetail.Relation.OriginalID == nil || *childDetail.Relation.OriginalID != parent {
		t.Errorf("child original = %v, want %s", childDetail.Relation.OriginalID, parent)
	}

	// Inheritance reaches through the chain, not just one level.
	grandchildDetail, _, err := repo.GetWork(grandchild)
	if err != nil {
		t.Fatal(err)
	}
	if grandchildDetail.Relation.Language == nil || *grandchildDetail.Relation.Language != "JPN" {
		t.Errorf("grandchild language = %v, want the inherited JPN", grandchildDetail.Relation.Language)
	}
	if grandchildDetail.Relation.OriginalID == nil || *grandchildDetail.Relation.OriginalID != parent {
		t.Errorf("grandchild original = %v, want %s", grandchildDetail.Relation.OriginalID, parent)
	}
}

func TestImportKeepsReferencesExternal(t *testing.T) {
	repo := newTestRepository(t)
	parent := seedWork(t, repo, "RJ00000001")
	child := code(t, "RJ00000009")
	importGroups(t, repo, parent, &works.ValidatedImport{
		Target:    parent,
		Relations: relationsGroup(t, works.RoleParent, nil, string(child)),
		Editions: &works.EditionsGroup{
			Source:    works.SourceDLsite,
			FetchedAt: mustTime(t, testStamp),
			Items:     []works.LanguageEdition{{VersionRootRJ: child, Language: "CHI_HANS"}},
		},
	})
	if _, found, err := repo.GetWork(child); err != nil || found {
		t.Fatalf("reference became a work: %v, %v", found, err)
	}
	page, err := repo.ListWorks(works.Filter{})
	if err != nil || len(page.Works) != 1 {
		t.Fatalf("catalog: %+v, %v", page, err)
	}
}

// TestReropeningADeclarationClearsTheInheritedValues is the regression for
// reading only the post-change evidence: the child is absent from the new
// graph, so only the graph as it was before still knows to recompute it.
func TestReropeningADeclarationClearsTheInheritedValues(t *testing.T) {
	repo := newTestRepository(t)
	parent := seedWork(t, repo, "RJ00000001")
	child := seedWork(t, repo, "RJ00000002")

	language := "JPN"
	importGroups(t, repo, parent, &works.ValidatedImport{
		Target: parent,
		Relations: &works.RelationsGroup{
			Source: works.SourceDLsite, FetchedAt: mustTime(t, testStamp),
			Role: works.RoleOriginal, Language: &language, ChildRJs: []works.WorkCode{child},
		},
	})
	before, _, err := repo.GetWork(child)
	if err != nil {
		t.Fatal(err)
	}
	if before.Relation.ParentID == nil {
		t.Fatal("the child did not inherit a parent to begin with")
	}

	// The parent re-reports with no children at all.
	later := "2026-09-28T10:00:00.000000000Z"
	importGroups(t, repo, parent, &works.ValidatedImport{
		Target: parent,
		Relations: &works.RelationsGroup{
			Source: works.SourceDLsite, FetchedAt: mustTime(t, later),
			Role: works.RoleOriginal, Language: &language,
		},
	})

	after, _, err := repo.GetWork(child)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Relation.IsEmpty() {
		t.Errorf("the child still holds %+v, want its inherited values cleared", after.Relation)
	}
}

// Independent reports preserve the child's relation after its parent leaves the catalog.
func TestWorkWithNoInboundEvidenceCanBeDeleted(t *testing.T) {
	repo := newTestRepository(t)
	parent := seedWork(t, repo, "RJ00000001")
	child := seedWork(t, repo, "RJ00000002")

	importGroups(t, repo, parent, &works.ValidatedImport{
		Target:    parent,
		Relations: relationsGroup(t, works.RoleParent, nil, string(child)),
	})
	derived, _, err := repo.GetWork(child)
	if err != nil {
		t.Fatal(err)
	}
	if derived.Relation.ParentID == nil {
		t.Fatal("the child was not derived to have a parent")
	}

	if refusal := repo.DeleteWork(parent); refusal != nil {
		t.Fatalf("a deletable parent was refused: %v", refusal)
	}

	if _, found, lookupErr := repo.GetWork(parent); lookupErr != nil || found {
		t.Errorf("the parent survived its own deletion: found=%v err=%v", found, lookupErr)
	}
	after, _, err := repo.GetWork(child)
	if err != nil {
		t.Fatal(err)
	}
	if after.Relation.ParentID == nil || *after.Relation.ParentID != parent {
		t.Errorf("the child lost its external parent after deletion: %+v", after.Relation)
	}
}

// Relationship references do not protect catalog membership; copies still do.
func TestDeleteAllowsRelationshipReferencesButProtectsCopies(t *testing.T) {
	repo := newTestRepository(t)
	original := seedWork(t, repo, "RJ00000001")
	translation := seedWork(t, repo, "RJ00000002")
	importGroups(t, repo, translation, &works.ValidatedImport{
		Target:    translation,
		Relations: relationsGroup(t, works.RoleChild, new(string(original))),
	})
	if refusal := repo.DeleteWork(original); refusal != nil {
		t.Fatal(refusal)
	}
	detail, _, err := repo.GetWork(translation)
	if err != nil || detail.Relation.ParentID == nil || *detail.Relation.ParentID != original {
		t.Fatalf("external parent lost: %+v, %v", detail, err)
	}

	// A bound copy holds it too.
	repo2 := newTestRepository(t)
	held := seedWork(t, repo2, "RJ00000003")
	library, err := repo2.CreateLibrary("music", "/music")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo2.DB().Exec(
		"INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at) VALUES (?, ?, ?, ?, ?)",
		"copy-1", string(held), library.ID, "RJ00000003", testStamp,
	); err != nil {
		t.Fatal(err)
	}
	if refusal := repo2.DeleteWork(held); refusal == nil || refusal.Code != works.CodeWorkInUse {
		t.Errorf("a work with a bound copy was deletable: %v", refusal)
	}
}

// TestUnnamedEntityKeepsItsIdentityAcrossRefreshes: an entity the source gave
// no number has its identity from (source, work, name), so a second import
// reuses the row instead of minting a new one.
func TestUnnamedEntityKeepsItsIdentityAcrossRefreshes(t *testing.T) {
	repo := newTestRepository(t)
	work := seedWork(t, repo, "RJ00000001")

	group := func(at string) *works.ValidatedImport {
		return &works.ValidatedImport{
			Target: work,
			Core: &works.CoreGroup{
				Source:    works.SourceDLsite,
				FetchedAt: mustTime(t, at),
				VoiceActors: []works.EntityInput{
					{Name: "伊ヶ崎綾香"},
				},
			},
		}
	}
	importGroups(t, repo, work, group(testStamp))
	first, _, err := repo.GetWork(work)
	if err != nil {
		t.Fatal(err)
	}
	importGroups(t, repo, work, group("2026-09-28T10:00:00.000000000Z"))
	second, _, err := repo.GetWork(work)
	if err != nil {
		t.Fatal(err)
	}

	if len(first.VoiceActors) != 1 || len(second.VoiceActors) != 1 {
		t.Fatalf("voice actors = %d then %d, want one each time", len(first.VoiceActors), len(second.VoiceActors))
	}
	if first.VoiceActors[0].ID != second.VoiceActors[0].ID {
		t.Errorf("the entity id drifted across refreshes: %s -> %s", first.VoiceActors[0].ID, second.VoiceActors[0].ID)
	}
}

// TestSourceRulesPerGroup: a lower-priority source never overwrites a
// higher-priority one, and the same source only moves forward in time.
func TestSourceRulesPerGroup(t *testing.T) {
	repo := newTestRepository(t)
	work := seedWork(t, repo, "RJ00000001")

	importGroups(t, repo, work, &works.ValidatedImport{
		Target: work,
		Core:   coreGroup(t, works.SourceDLsite, "from dlsite"),
	})
	outcome := importGroups(t, repo, work, &works.ValidatedImport{
		Target: work,
		Core:   coreGroup(t, works.SourceASMRone, "from asmr.one"),
	})
	if len(outcome.Ignored) != 1 || outcome.Ignored[0].Reason != works.ReasonLowerPrioritySource {
		t.Errorf("outcome = %+v, want the asmr.one group ignored", outcome)
	}
	detail, _, err := repo.GetWork(work)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Title == nil || *detail.Title != "from dlsite" {
		t.Errorf("title = %v, want the DLsite one to have survived", detail.Title)
	}

	// The same source at the same fetch time changes nothing either: only a
	// strictly newer fetch moves a group forward.
	outcome = importGroups(t, repo, work, &works.ValidatedImport{
		Target: work,
		Core:   coreGroup(t, works.SourceDLsite, "rewritten"),
	})
	if len(outcome.Applied) != 0 || len(outcome.Ignored) != 1 {
		t.Errorf("a same-time re-import was applied: %+v", outcome)
	}
	if outcome.Ignored[0].Reason != works.ReasonStaleFetchedAt {
		t.Errorf("reason = %q, want %q", outcome.Ignored[0].Reason, works.ReasonStaleFetchedAt)
	}
}

// TestUnsubmittedGroupsAreUntouched: a submission carries only what it fetched,
// and the groups it does not mention keep their stored values.
func TestUnsubmittedGroupsAreUntouched(t *testing.T) {
	repo := newTestRepository(t)
	work := seedWork(t, repo, "RJ00000001")

	language := "JPN"
	importGroups(t, repo, work, &works.ValidatedImport{
		Target:     work,
		Core:       coreGroup(t, works.SourceDLsite, "kept"),
		Statistics: &works.StatisticsGroup{FetchedAt: mustTime(t, testStamp), DLCount: new(7)},
		Relations: &works.RelationsGroup{
			Source:    works.SourceDLsite,
			FetchedAt: mustTime(t, testStamp),
			Role:      works.RoleOriginal,
			Language:  &language,
		},
	})

	// A later submission refreshes only the sales figure.
	importGroups(t, repo, work, &works.ValidatedImport{
		Target:     work,
		Statistics: &works.StatisticsGroup{FetchedAt: mustTime(t, "2026-09-28T10:00:00.000000000Z"), DLCount: new(99)},
	})

	detail, _, err := repo.GetWork(work)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Title == nil || *detail.Title != "kept" {
		t.Errorf("title = %v, want the unsubmitted core group untouched", detail.Title)
	}
	if detail.Relation.Role == nil || *detail.Relation.Role != works.RoleOriginal {
		t.Errorf("role = %v, want the unsubmitted relations group untouched", detail.Relation.Role)
	}
	if detail.DLCount == nil || *detail.DLCount != 99 {
		t.Errorf("dl_count = %v, want the refreshed 99", detail.DLCount)
	}
}

// TestRelationsRefusalsRollBackEverything: a structurally illegal relation
// refuses the whole submission, including the groups that came with it.
func TestRelationsRefusalsRollBackEverything(t *testing.T) {
	repo := newTestRepository(t)
	first := seedWork(t, repo, "RJ00000001")
	second := seedWork(t, repo, "RJ00000002")

	// Make the second work an original of a third, so naming it as an original
	// makes a chain rather than a single link.
	third := seedWork(t, repo, "RJ00000003")
	importGroups(t, repo, second, &works.ValidatedImport{
		Target: second,
		Relations: &works.RelationsGroup{
			Source: works.SourceDLsite, FetchedAt: mustTime(t, testStamp),
			Role: works.RoleChild, OriginalRJ: &third,
		},
	})

	_, err := repo.ImportMetadata(&works.ValidatedImport{
		Target: first,
		Core:   coreGroup(t, works.SourceDLsite, "should not survive"),
		Relations: &works.RelationsGroup{
			Source: works.SourceDLsite, FetchedAt: mustTime(t, testStamp),
			Role: works.RoleChild, OriginalRJ: &second,
		},
	}, mustTime(t, testStamp))
	if err == nil {
		t.Fatal("an original chain was accepted")
	}
	if refusal, ok := works.AsError(
		err,
	); !ok ||
		refusal.Code != works.CodeRelationCycle && refusal.Code != works.CodeRelationOriginalChain {
		t.Errorf("refusal = %v, want a relation refusal", err)
	}

	detail, _, err := repo.GetWork(first)
	if err != nil {
		t.Fatal(err)
	}
	// The core group that travelled with the refused relation group wrote
	// nothing: the seeded title is still the one on the row.
	if detail.Title == nil || *detail.Title != "title of RJ00000001" {
		t.Errorf("title = %v, want the refused submission to have written nothing", detail.Title)
	}
}

//go:fix inline

//go:fix inline

// TestRepeatedEntityInOnePayloadIsListedOnce: an upstream payload can name the
// same credited actor twice, and the work's own list must not collide with
// itself over it.
func TestRepeatedEntityInOnePayloadIsListedOnce(t *testing.T) {
	repo := newTestRepository(t)
	work := seedWork(t, repo, "RJ00000001")
	actor := works.EntityInput{SourceID: new("17180"), Name: "伊ヶ崎綾香"}

	importGroups(t, repo, work, &works.ValidatedImport{
		Target: work,
		Core: &works.CoreGroup{
			Source:      works.SourceDLsite,
			FetchedAt:   mustTime(t, testStamp),
			VoiceActors: []works.EntityInput{actor, actor},
			Tags:        []works.EntityInput{{SourceID: new("497"), Name: "ASMR"}},
		},
	})

	detail, _, err := repo.GetWork(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.VoiceActors) != 1 {
		t.Errorf("voice actors = %d, want the repeated mention listed once", len(detail.VoiceActors))
	}
	if len(detail.SourceTags) != 1 {
		t.Errorf("tags = %d, want one", len(detail.SourceTags))
	}
}

func TestParentNumberLookupUsesChineseCopiesInTheSelectedLibrary(t *testing.T) {
	repo := newTestRepository(t)
	parent := code(t, "RJ01526615")
	first := seedWork(t, repo, "RJ01526629")
	second := seedWork(t, repo, "RJ01535135")
	english := seedWork(t, repo, "RJ01535136")
	orphan := seedWork(t, repo, "RJ01535137")
	libA, err := repo.CreateLibrary("A", "/a")
	if err != nil {
		t.Fatal(err)
	}
	libB, err := repo.CreateLibrary("B", "/b")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id       works.WorkCode
		language string
		library  string
	}{
		{first, "CHI_HANS", libA.ID}, {second, "CHI_HANS", libB.ID},
		{english, "ENG", libA.ID}, {orphan, "CHI_HANS", ""},
	} {
		importGroups(t, repo, row.id, &works.ValidatedImport{Target: row.id,
			Relations: &works.RelationsGroup{Source: works.SourceDLsite, FetchedAt: mustTime(t, testStamp),
				Role: works.RoleChild, Language: &row.language, ParentRJ: &parent},
		})
		if row.library != "" {
			if _, err := repo.DB().
				Exec("INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at) VALUES (?, ?, ?, ?, ?)",
					string(row.id), string(row.id), row.library, string(row.id), testStamp); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		library []string
		count   int
	}{
		{nil, 2}, {[]string{libA.ID}, 1}, {[]string{libB.ID}, 1},
	} {
		page, err := repo.ListWorks(works.Filter{Query: new(string(parent)), Libraries: tc.library})
		if err != nil || len(page.Works) != tc.count {
			t.Fatalf("parent lookup: %+v, %v", page, err)
		}
	}
}

func TestAcceptedCoreRefreshesSourceTagNamesWithoutChangingIdentity(t *testing.T) {
	repo := newTestRepository(t)
	id := seedWork(t, repo, "RJ00000001")
	for i, name := range []string{"日本語タグ", "简体中文标签"} {
		importGroups(t, repo, id, &works.ValidatedImport{Target: id, Core: &works.CoreGroup{
			Source: works.SourceDLsite, FetchedAt: mustTime(t, testStamp).Add(time.Duration(i) * time.Second),
			Tags: []works.EntityInput{{SourceID: new("68"), Name: name}},
		}})
		detail, _, err := repo.GetWork(id)
		if err != nil || len(detail.SourceTags) != 1 || detail.SourceTags[0].Name != name {
			t.Fatalf("tags: %+v, %v", detail, err)
		}
	}
	var count int
	if err := repo.DB().
		QueryRow("SELECT COUNT(*) FROM tags WHERE source_tag_id = '68'").
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatalf("tag identity drifted: %d, %v", count, err)
	}
}

func TestWorkLanguageImportAndFilterStayIndependentOfRelations(t *testing.T) {
	repo := newTestRepository(t)
	id := seedWork(t, repo, "RJ00000001")
	for i, language := range []*string{new("NM"), new("CHI_HANS"), nil} {
		at := mustTime(t, testStamp).Add(time.Duration(i) * time.Second).Format(works.TimestampFormat)
		req := works.ImportRequest{Core: &works.ImportCore{Source: "dlsite", FetchedAt: at, Language: language}}
		if i == 0 {
			req.Relations = &works.ImportRelations{Source: "dlsite", FetchedAt: at, Role: "original"}
		}
		validated, refusal := works.ValidateImport(id, req)
		if refusal != nil {
			t.Fatal(refusal)
		}
		importGroups(t, repo, id, validated)
		detail, found, err := repo.GetWork(id)
		if err != nil || !found {
			t.Fatalf("detail: %v, %v", found, err)
		}
		if language == nil && detail.Language != nil ||
			language != nil && (detail.Language == nil || *detail.Language != *language) {
			t.Fatalf("language: got %v, want %v", detail.Language, language)
		}
		if detail.Relation.Language != nil {
			t.Fatal("work language changed relation language")
		}
		for _, filter := range []string{"NM", "CHI_HANS"} {
			page, err := repo.ListWorks(works.Filter{Languages: []string{filter}})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if language != nil && filter == *language {
				want = 1
			}
			if page.Total != want || len(page.Works) != want {
				t.Fatalf("filter %s: %+v", filter, page)
			}
			if want == 1 && (page.Works[0].Language == nil || *page.Works[0].Language != filter) {
				t.Fatal("list lost language")
			}
		}
	}
	_, refusal := works.ValidateImport(id, works.ImportRequest{
		Core: &works.ImportCore{Source: "dlsite", FetchedAt: testStamp, Language: new("DOT")},
	})
	if refusal == nil {
		t.Fatal("translation provenance accepted as language")
	}
}
