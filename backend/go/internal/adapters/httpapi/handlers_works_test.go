package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"github.com/onsei/organizer/backend/internal/adapters/filesystem"

	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/works"
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
// selects, an unknown value that is its own case, and a cursor that walks the
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
					"INSERT INTO work_user_data (work_id, rating, favorite, note) VALUES (?, 9, 1, '')",
					spec.id,
				); err != nil {
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

	// Paging with a cursor visits every row exactly once, in order.
	seen := []string{}
	path := "/api/v1/works?limit=1&sort=release_date&direction=asc"
	for range 5 {
		pageResponse := doRequest(t, engine, http.MethodGet, path, nil, nil)
		if pageResponse.Code != http.StatusOK {
			t.Fatalf("status = %d (body=%s)", pageResponse.Code, pageResponse.Body.String())
		}
		var page struct {
			Works []struct {
				RJ string `json:"rj"`
			} `json:"works"`
			NextCursor string `json:"next_cursor"`
		}
		if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, summary := range page.Works {
			seen = append(seen, summary.RJ)
		}
		if page.NextCursor == "" {
			break
		}
		path = "/api/v1/works?limit=1&sort=release_date&direction=asc&cursor=" + page.NextCursor
	}
	assertSequence(t, seen, []string{"RJ00000001", "RJ00000002", "RJ00000003"})

	// A cursor from one query is refused by another: it describes a position in
	// a sequence that no longer exists.
	first := doRequest(t, engine, http.MethodGet, "/api/v1/works?limit=1&sort=release_date&direction=asc", nil, nil)
	var page struct {
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	w = doRequest(t, engine, http.MethodGet,
		"/api/v1/works?limit=1&sort=release_date&direction=asc&favorite=true&cursor="+page.NextCursor, nil, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if code, _ := errorEnvelope(t, w); code != works.CodeCursorMismatch {
		t.Errorf("code = %q, want %q", code, works.CodeCursorMismatch)
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
// repeated and none is skipped. Two unknown dates make the walk continue from
// inside the unknown block, the cursor shape that carries no sort value.
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
		seen := []string{}
		path := "/api/v1/works?limit=1&sort=release_date&direction=" + direction
		for range 7 {
			response := doRequest(t, engine, http.MethodGet, path, nil, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("%s: status = %d (body=%s)", direction, response.Code, response.Body.String())
			}
			var page struct {
				Works []struct {
					RJ string `json:"rj"`
				} `json:"works"`
				NextCursor string `json:"next_cursor"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			for _, summary := range page.Works {
				seen = append(seen, summary.RJ)
			}
			if page.NextCursor == "" {
				break
			}
			path = "/api/v1/works?limit=1&sort=release_date&direction=" + direction + "&cursor=" + page.NextCursor
		}
		assertSequence(t, seen, want)
	}

	// A release-date range never contains a work whose date is unknown.
	assertWorks(t,
		doRequest(t, engine, http.MethodGet,
			"/api/v1/works?sort=release_date&released_from=2000-01-01&released_to=2030-01-01", nil, nil),
		"RJ00000001", "RJ00000002", "RJ00000003")
}
