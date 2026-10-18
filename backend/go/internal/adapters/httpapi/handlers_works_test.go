package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"

	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/works"
)

// wireWorks installs the catalog entry over the test repository. The
// content manifest reads entries through the same repository and resolves
// members on disk the way direct file management does, so a test that wants
// an online copy creates the member directory under its library root.
func wireWorks(d *Dependencies, fixture *sqlite.Repository) {
	d.Works = works.NewService(fixture, fixture, filesystem.ResolveMember, filesystem.ResolveMemberFile, "test")
}

const stamp = "2026-09-27T10:00:00.000000000Z"

// seedHTTPWork inserts a work row the way a scan would have: found on disk.
func seedHTTPWork(t *testing.T, fixture *sqlite.Repository, raw, title string) string {
	t.Helper()
	if _, err := fixture.DB().Exec(`
		INSERT INTO works (id, title, collected_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, raw, title, stamp, stamp, stamp); err != nil {
		t.Fatalf("seed %s: %v", raw, err)
	}
	return raw
}

// TestWorksListRequiresNoUnknownParameters: a parameter the catalog does not
// understand is a 400, not something quietly ignored.
func TestWorksListRequiresNoUnknownParameters(t *testing.T) {
	engine := newTestServer(t, wireWorks)

	w := doRequest(t, engine, http.MethodGet, "/api/v1/works?nonsense=1", nil, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	code, _ := errorEnvelope(t, w)
	if code != works.CodeInvalidArgument {
		t.Errorf("code = %q, want %q", code, works.CodeInvalidArgument)
	}
}

// TestWorksListValidatesThePageSize: a bad limit is refused rather than
// silently replaced by the default or clamped.
func TestWorksListValidatesThePageSize(t *testing.T) {
	engine := newTestServer(t, wireWorks)
	for _, raw := range []string{"abc", "0", "-1", "201", "1.5"} {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/works?limit="+raw, nil, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("limit=%s: status = %d, want 400", raw, w.Code)
			continue
		}
		if code, _ := errorEnvelope(t, w); code != works.CodeInvalidPage {
			t.Errorf("limit=%s: code = %q, want %q", raw, code, works.CodeInvalidPage)
		}
	}
	// An absent limit takes the default and succeeds.
	if w := doRequest(t, engine, http.MethodGet, "/api/v1/works", nil, nil); w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestWorksListRequiresAMatchMode: a set filter without a match mode is refused,
// because choosing one on the caller's behalf answers a different question.
func TestWorksListRequiresAMatchMode(t *testing.T) {
	engine := newTestServer(t, wireWorks)
	for _, param := range []string{"voice_actor=va-1", "source_tag=tag-1", "user_tag=ut-1"} {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/works?"+param, nil, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", param, w.Code)
			continue
		}
		if code, _ := errorEnvelope(t, w); code != works.CodeMatchModeRequired {
			t.Errorf("%s: code = %q, want %q", param, code, works.CodeMatchModeRequired)
		}
	}
}

// TestWorksListFiltersAndPages is the ordinary listing end to end: a filter that
// selects, an unknown value that is its own case, and numbered pages that walk the
// whole set without repeating or skipping a row.
func TestWorksListFiltersAndPages(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for i, spec := range []struct{ id, title, release string }{
			{"RJ00000001", "alpha", "2025-01-01"},
			{"RJ00000002", "beta", "2025-06-01"},
			{"RJ00000003", "gamma", "2025-12-01"},
		} {
			seedHTTPWork(t, fixture, spec.id, spec.title)
			if _, err := fixture.DB().Exec(
				"UPDATE works SET release_date = ? WHERE id = ?", spec.release, spec.id,
			); err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				if _, err := fixture.DB().Exec(
					"INSERT INTO work_user_data (work_id, rating, note) VALUES (?, 9, '')",
					spec.id,
				); err != nil {
					t.Fatal(err)
				}
				if _, err := fixture.DB().
					Exec("INSERT INTO collection_works(collection_id,work_id,position) VALUES ('favorites',?,0)", spec.id); err != nil {
					t.Fatal(err)
				}
			}
		}
		// The three works cover the three adult-flag states: reported false,
		// reported true, and not reported at all.
		if _, err := fixture.DB().Exec("UPDATE works SET nsfw = 1 WHERE id = 'RJ00000002'"); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.DB().Exec("UPDATE works SET nsfw = 0 WHERE id = 'RJ00000003'"); err != nil {
			t.Fatal(err)
		}
	})

	// A literal substring over the title.
	w := doRequest(t, engine, http.MethodGet, "/api/v1/works?q=bet", nil, nil)
	assertWorks(t, w, "RJ00000002")

	// A query that is itself a number matches that work exactly, so the short
	// spelling finds the row stored in the canonical one - here in lower case,
	// which the literal search alone would never match.
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?q=rj000001", nil, nil)
	assertWorks(t, w, "RJ00000001")

	// A query that matches nothing is an empty page, not an error.
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?q=nothing-matches", nil, nil)
	assertWorks(t, w)

	// An unknown adult flag is its own value, not a synonym for false.
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?nsfw=unknown", nil, nil)
	assertWorks(t, w, "RJ00000001")
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?nsfw=false", nil, nil)
	assertWorks(t, w, "RJ00000003")
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?nsfw=true", nil, nil)
	assertWorks(t, w, "RJ00000002")

	// The rating range and the not-rated switch union inside their dimension:
	// asking for "9 and up, or not rated" is not an empty set.
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?rating_from=9&unrated=true", nil, nil)
	assertWorks(t, w, "RJ00000001", "RJ00000002", "RJ00000003")
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?rating_from=9", nil, nil)
	assertWorks(t, w, "RJ00000001")
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?unrated=true", nil, nil)
	assertWorks(t, w, "RJ00000002", "RJ00000003")
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?favorite=true", nil, nil)
	assertWorks(t, w, "RJ00000001")

	// Release range endpoints are inclusive.
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works?released_from=2025-06-01&released_to=2025-06-01", nil, nil)
	assertWorks(t, w, "RJ00000002")

	// Numbered pages visit every row exactly once, in order, and each one
	// carries the same total.
	assertSequence(t, walkPages(t, engine, "/api/v1/works?limit=1&sort=release_date&direction=asc", 3),
		[]string{"RJ00000001", "RJ00000002", "RJ00000003"})
	// A page past the end is empty but still says how many works there are.
	assertWorks(t, doRequest(t, engine, http.MethodGet, "/api/v1/works?limit=2&page=9", nil, nil))
}

// walkPages reads pages 1, 2, … of one listing until the total is covered,
// checking that every page reports the same total.
func walkPages(t *testing.T, engine http.Handler, base string, wantTotal int) []string {
	t.Helper()
	seen := []string{}
	for number := 1; ; number++ {
		response := doRequest(t, engine, http.MethodGet, base+"&page="+strconv.Itoa(number), nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("page %d: status = %d (body=%s)", number, response.Code, response.Body.String())
		}
		var page struct {
			Works []struct {
				RJ string `json:"rj"`
			} `json:"works"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Total != wantTotal {
			t.Fatalf("page %d: total = %d, want %d", number, page.Total, wantTotal)
		}
		if len(page.Works) == 0 {
			return seen
		}
		for _, summary := range page.Works {
			seen = append(seen, summary.RJ)
		}
	}
}

// TestWorksListValidatesThePageNumber: an absent page is the first, while a
// supplied one has to be a whole number of at least 1.
func TestWorksListValidatesThePageNumber(t *testing.T) {
	engine := newTestServer(t, wireWorks)
	for _, raw := range []string{"abc", "0", "-1", "1.5"} {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/works?page="+raw, nil, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("page=%s: status = %d, want 400", raw, w.Code)
			continue
		}
		if code, _ := errorEnvelope(t, w); code != works.CodeInvalidPage {
			t.Errorf("page=%s: code = %q, want %q", raw, code, works.CodeInvalidPage)
		}
	}
	if w := doRequest(t, engine, http.MethodGet, "/api/v1/works", nil, nil); w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// assertWorks checks which works a listing answered with, in order.
func assertWorks(t *testing.T, w *httptest.ResponseRecorder, want ...string) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var page struct {
		Works []struct {
			RJ string `json:"rj"`
		} `json:"works"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := make([]string, 0, len(page.Works))
	for _, summary := range page.Works {
		got = append(got, summary.RJ)
	}
	assertSequence(t, got, want)
}

func assertSequence(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sequence = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("sequence = %v, want %v", got, want)
		}
	}
}

// TestUnknownReleaseDatesAlwaysSortLast walks the whole set in both directions
// across page boundaries: an unknown release date is last either way, no row is
// repeated and none is skipped. Two unknown dates put a page boundary inside
// the unknown block, where the work number alone orders the rows.
func TestUnknownReleaseDatesAlwaysSortLast(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for _, spec := range []struct{ id, release string }{
			{"RJ00000001", "2025-01-01"},
			{"RJ00000002", "2025-06-01"},
			{"RJ00000003", "2025-12-01"},
			{"RJ00000004", ""},
			{"RJ00000005", ""},
		} {
			seedHTTPWork(t, fixture, spec.id, "titled")
			if spec.release == "" {
				continue
			}
			if _, err := fixture.DB().Exec(
				"UPDATE works SET release_date = ? WHERE id = ?", spec.release, spec.id,
			); err != nil {
				t.Fatal(err)
			}
		}
	})

	for _, direction := range []string{"asc", "desc"} {
		want := []string{"RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004", "RJ00000005"}
		if direction == "desc" {
			// The known dates reverse; the unknown ones stay last, by number.
			want = []string{"RJ00000003", "RJ00000002", "RJ00000001", "RJ00000004", "RJ00000005"}
		}
		seen := walkPages(t, engine, "/api/v1/works?limit=1&sort=release_date&direction="+direction, 5)
		assertSequence(t, seen, want)
	}

	// A release-date range never contains a work whose date is unknown.
	assertWorks(t,
		doRequest(t, engine, http.MethodGet,
			"/api/v1/works?sort=release_date&released_from=2000-01-01&released_to=2030-01-01", nil, nil),
		"RJ00000001", "RJ00000002", "RJ00000003")
}

// TestWorkDetailAnswersTheEditionTreeAndCopyDirIDs: the detail resolves the
// edition lines around a translation (held flags, placeholders as works) and
// gives each copy the navigation id of its folder.
func TestWorkDetailAnswersTheEditionTreeAndCopyDirIDs(t *testing.T) {
	engine, repo := serverOnDB(t, filepath.Join(t.TempDir(), "tree.db"))
	t.Cleanup(func() { _ = repo.Close() })
	libID, root := treeLibrary(t, engine, repo, "Music")
	handler := NewServer(Dependencies{
		Library:     library.NewService(repo, nil, repo, filesystem.ResolveMember),
		Works:       works.NewService(repo, nil, nil, nil, "test"),
		CORSOrigins: []string{},
		Version:     "dev",
	})
	for _, id := range []string{"RJ01287230", "RJ01289610", "RJ01289611"} {
		seedHTTPWork(t, repo, id, id)
	}
	for _, q := range []string{
		`INSERT INTO work_copies(id, work_id, library_id, rel_path, created_at) VALUES('c1','RJ01289611','` + libID + `','tr dir','now')`,
		`INSERT INTO work_copies(id, work_id, library_id, rel_path, created_at) VALUES('c2','RJ01287230','` + libID + `','orig dir','now')`,
		`INSERT INTO work_language_editions(work_id, version_root_rj, language, label, display_order) VALUES
			('RJ01289611','RJ01289610','CHI_HANS','简中',5),('RJ01289611','RJ01291590','CHI_HANT','繁中',7),
			('RJ01289611','RJ01299999','ENG','English',1)`,
		`INSERT INTO work_translations(work_id, role, language, original_work_id, parent_work_id) VALUES
			('RJ01289611','child','CHI_HANS','RJ01287230','RJ01289610')`,
	} {
		if _, err := repo.DB().Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	w := doRequest(t, handler, http.MethodGet, "/api/v1/works/RJ01289611", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", w.Code, w.Body.String())
	}
	var detail struct {
		EditionTree []struct {
			Language string `json:"language"`
			Held     bool   `json:"held"`
			Root     struct {
				RJ     string `json:"rj"`
				Role   string `json:"role"`
				IsWork bool   `json:"is_work"`
				Held   bool   `json:"held"`
			} `json:"root"`
			Translations []struct {
				RJ   string `json:"rj"`
				Held bool   `json:"held"`
			} `json:"translations"`
		} `json:"edition_tree"`
		Copies []struct {
			DirID string `json:"dir_id"`
		} `json:"copies"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	tree := detail.EditionTree
	if len(tree) != 3 || tree[0].Language != "JPN" || tree[1].Language != "CHI_HANS" || tree[2].Language != "CHI_HANT" {
		t.Fatalf("tree = %+v, want JPN, CHI_HANS, CHI_HANT", tree)
	}
	if !tree[0].Root.Held || tree[0].Root.Role != "original" {
		t.Errorf("original = %+v", tree[0].Root)
	}
	if tree[1].Root.RJ != "RJ01289610" || !tree[1].Root.IsWork || tree[1].Root.Held || !tree[1].Held {
		t.Errorf("edition page = %+v held=%v", tree[1].Root, tree[1].Held)
	}
	if len(tree[1].Translations) != 1 || tree[1].Translations[0].RJ != "RJ01289611" || !tree[1].Translations[0].Held {
		t.Errorf("translations = %+v", tree[1].Translations)
	}
	if tree[2].Root.IsWork || tree[2].Held || len(tree[2].Translations) != 0 {
		t.Errorf("unfetched page = %+v", tree[2])
	}
	if want := library.DirID(libID, root, "tr dir"); len(detail.Copies) != 1 || detail.Copies[0].DirID != want {
		t.Errorf("copies = %+v, want dir_id %s", detail.Copies, want)
	}
}
