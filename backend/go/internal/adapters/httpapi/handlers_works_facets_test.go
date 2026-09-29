package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/works"
)

// execAll runs fixture statements, failing the test on the first error.
func execAll(t *testing.T, fixture *sqlite.Repository, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := fixture.DB().Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// TestRatingOrderPutsUnratedWorksLast walks the rating order across page
// boundaries in both directions. Several unrated works make the walk continue
// from inside the unknown block, which is the cursor shape that carries no
// value.
func TestRatingOrderPutsUnratedWorksLast(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for _, id := range []string{"RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004", "RJ00000005"} {
			seedHTTPWork(t, fixture, id, "titled")
		}
		execAll(t, fixture,
			"INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000002', 3)",
			"INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000004', 9)",
			"INSERT INTO work_user_data (work_id, rating) VALUES ('RJ00000005', 9)",
			// A favorite with no rating is a user-data row that is still unrated.
			"INSERT INTO work_user_data (work_id, favorite) VALUES ('RJ00000003', 1)",
		)
	})

	for direction, want := range map[string][]string{
		// Equal ratings break by work number; unrated rows come last either way.
		"desc": {"RJ00000004", "RJ00000005", "RJ00000002", "RJ00000001", "RJ00000003"},
		"asc":  {"RJ00000002", "RJ00000004", "RJ00000005", "RJ00000001", "RJ00000003"},
	} {
		seen := []string{}
		path := "/api/v1/works?limit=1&sort=rating&direction=" + direction
		for range 8 {
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
			path = "/api/v1/works?limit=1&sort=rating&direction=" + direction + "&cursor=" + page.NextCursor
		}
		assertSequence(t, seen, want)
	}
}

// TestFacetListsFeedTheListingFilters: the circle and voice-actor lists name
// exactly the entities listed works carry, with their counts, and a library
// scopes both the entities and the counts.
func TestFacetListsFeedTheListingFilters(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for _, id := range []string{"RJ00000001", "RJ00000002", "RJ00000003"} {
			seedHTTPWork(t, fixture, id, "titled")
		}
		execAll(
			t,
			fixture,
			"INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib-a', 'A', '/a', '/a')",
			"INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at) VALUES "+
				"('c1', 'RJ00000001', 'lib-a', 'RJ00000001', '"+stamp+"')",
			"INSERT INTO circles (id, source, source_circle_id, name) VALUES ('ci-1', 'dlsite', 'RG1', 'mete0r')",
			"INSERT INTO circles (id, source, source_circle_id, name) VALUES ('ci-2', 'dlsite', 'RG2', 'Aqua')",
			"UPDATE works SET circle_id = 'ci-1' WHERE id IN ('RJ00000001', 'RJ00000002')",
			"UPDATE works SET circle_id = 'ci-2' WHERE id = 'RJ00000003'",
			"INSERT INTO voice_actors (id, source, source_voice_actor_id, name) VALUES ('va-1', 'dlsite', '1', '伊ヶ崎綾香')",
			"INSERT INTO voice_actors (id, source, source_voice_actor_id, name) VALUES ('va-2', 'dlsite', '2', '沢城みゆき')",
			// A voice actor only a relation placeholder carries is not listed.
			"INSERT INTO voice_actors (id, source, source_voice_actor_id, name) VALUES ('va-3', 'dlsite', '3', 'unseen')",
			"INSERT INTO works (id, created_at, updated_at) VALUES ('RJ00000009', '"+stamp+"', '"+stamp+"')",
			"INSERT INTO work_voice_actors (work_id, voice_actor_id, position) VALUES ('RJ00000001', 'va-1', 0)",
			"INSERT INTO work_voice_actors (work_id, voice_actor_id, position) VALUES ('RJ00000001', 'va-2', 1)",
			"INSERT INTO work_voice_actors (work_id, voice_actor_id, position) VALUES ('RJ00000002', 'va-2', 0)",
			"INSERT INTO work_voice_actors (work_id, voice_actor_id, position) VALUES ('RJ00000009', 'va-3', 0)",
		)
	})

	type facet struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		WorkCount int    `json:"work_count"`
	}
	read := func(path, key string) []facet {
		t.Helper()
		response := doRequest(t, engine, http.MethodGet, path, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status = %d (body=%s)", path, response.Code, response.Body.String())
		}
		var body map[string][]facet
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body[key]
	}
	assertFacets := func(got []facet, want ...facet) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("facets = %+v, want %+v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("facets = %+v, want %+v", got, want)
			}
		}
	}

	// Ordered by name; counts are listed works.
	assertFacets(read("/api/v1/circles", "circles"),
		facet{"ci-2", "Aqua", 1}, facet{"ci-1", "mete0r", 2})
	assertFacets(read("/api/v1/voice-actors", "voice_actors"),
		facet{"va-1", "伊ヶ崎綾香", 1}, facet{"va-2", "沢城みゆき", 2})

	// The library scope narrows the entities and their counts together.
	assertFacets(read("/api/v1/circles?library=lib-a", "circles"), facet{"ci-1", "mete0r", 1})
	assertFacets(read("/api/v1/voice-actors?library=lib-a", "voice_actors"),
		facet{"va-1", "伊ヶ崎綾香", 1}, facet{"va-2", "沢城みゆき", 1})

	// A listed id is exactly what the listing parameter takes.
	assertWorks(t, doRequest(t, engine, http.MethodGet,
		"/api/v1/works?voice_actor=va-2&voice_actor_match=any", nil, nil),
		"RJ00000001", "RJ00000002")

	// Anything but the library scope is refused, as the listing refuses it.
	response := doRequest(t, engine, http.MethodGet, "/api/v1/circles?q=aqua", nil, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if code, _ := errorEnvelope(t, response); code != works.CodeInvalidArgument {
		t.Errorf("code = %q, want %q", code, works.CodeInvalidArgument)
	}
}
