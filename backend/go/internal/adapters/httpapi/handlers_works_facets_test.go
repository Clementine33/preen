package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

// TestTagIndexCombinesBothVocabularies: /tags lists the source's tags and the
// user's own under one scope, and the combined `tag` filter treats one group as
// one picked tag (any of its ids, either vocabulary) while tag_match combines
// the groups.
func TestTagIndexCombinesBothVocabularies(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for _, id := range []string{"RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004"} {
			seedHTTPWork(t, fixture, id, "titled")
		}
		execAll(t, fixture,
			"INSERT INTO libraries (id, name, root_path, root_path_key) VALUES ('lib-a', 'A', '/a', '/a')",
			"INSERT INTO work_copies (id, work_id, library_id, rel_path, created_at) VALUES "+
				"('c1', 'RJ00000002', 'lib-a', 'RJ00000002', '"+stamp+"')",
			"INSERT INTO circles (id, source, source_circle_id, name) VALUES ('ci-1', 'dlsite', 'RG1', 'mete0r')",
			"UPDATE works SET circle_id = 'ci-1' WHERE id = 'RJ00000001'",
			"INSERT INTO tags (id, source, source_tag_id, name) VALUES ('s-asmr', 'dlsite', '1', 'ASMR')",
			"INSERT INTO tags (id, source, source_tag_id, name) VALUES ('s-ear', 'dlsite', '2', '掏耳')",
			"INSERT INTO work_tags (work_id, tag_id, position) VALUES ('RJ00000001', 's-asmr', 0)",
			"INSERT INTO work_tags (work_id, tag_id, position) VALUES ('RJ00000002', 's-asmr', 0)",
			"INSERT INTO work_tags (work_id, tag_id, position) VALUES ('RJ00000002', 's-ear', 1)",
			"INSERT INTO work_tags (work_id, tag_id, position) VALUES ('RJ00000003', 's-ear', 0)",
			// The user's own tag of the same name: one choice in the index.
			"INSERT INTO user_tags (id, name) VALUES ('u-asmr', 'ASMR')",
			"INSERT INTO user_tags (id, name) VALUES ('u-mine', 'mine')",
			"INSERT INTO work_user_tags (work_id, tag_id) VALUES ('RJ00000004', 'u-asmr')",
			"INSERT INTO work_user_tags (work_id, tag_id) VALUES ('RJ00000001', 'u-mine')",
		)
	})

	type facet struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		WorkCount int    `json:"work_count"`
	}
	read := func(path string) map[string][]facet {
		t.Helper()
		response := doRequest(t, engine, http.MethodGet, path, nil, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status = %d (body=%s)", path, response.Code, response.Body.String())
		}
		var body map[string][]facet
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	all := read("/api/v1/tags")
	if want := []facet{{"s-asmr", "ASMR", 2}, {"s-ear", "掏耳", 2}}; !slices.Equal(all["source_tags"], want) {
		t.Errorf("source_tags = %+v, want %+v", all["source_tags"], want)
	}
	if want := []facet{{"u-asmr", "ASMR", 1}, {"u-mine", "mine", 1}}; !slices.Equal(all["user_tags"], want) {
		t.Errorf("user_tags = %+v, want %+v", all["user_tags"], want)
	}
	// The library scope narrows both vocabularies; an empty one is still a list.
	scoped := read("/api/v1/tags?library=lib-a")
	if want := []facet{{"s-asmr", "ASMR", 1}, {"s-ear", "掏耳", 1}}; !slices.Equal(scoped["source_tags"], want) {
		t.Errorf("scoped source_tags = %+v, want %+v", scoped["source_tags"], want)
	}
	if scoped["user_tags"] == nil || len(scoped["user_tags"]) != 0 {
		t.Errorf("scoped user_tags = %#v, want []", scoped["user_tags"])
	}

	list := func(query string) *httptest.ResponseRecorder {
		return doRequest(t, engine, http.MethodGet, "/api/v1/works?"+query, nil, nil)
	}
	asmr := "s:s-asmr|u:u-asmr"
	// One group: a work carrying the tag in either vocabulary.
	assertWorks(t, list("tag="+asmr+"&tag_match=any"), "RJ00000001", "RJ00000002", "RJ00000004")
	assertWorks(t, list("tag="+asmr+",s:s-ear&tag_match=all"), "RJ00000002")
	assertWorks(t, list("tag="+asmr+",s:s-ear&tag_match=any"),
		"RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004")
	// Across vocabularies, OR is now expressible; AND finds nobody here.
	assertWorks(t, list("tag=s:s-ear,u:u-mine&tag_match=any"), "RJ00000001", "RJ00000002", "RJ00000003")
	assertWorks(t, list("tag=s:s-ear,u:u-mine&tag_match=all"))
	// The tag dimension still intersects with the others.
	assertWorks(t, list("tag=s:s-ear,u:u-mine&tag_match=any&circle=ci-1"), "RJ00000001")

	for query, code := range map[string]string{
		"tag=" + asmr:                     works.CodeMatchModeRequired,
		"tag_match=any":                   works.CodeInvalidArgument,
		"tag=x:s-asmr&tag_match=any":      works.CodeInvalidArgument,
		"tag=s:&tag_match=any":            works.CodeInvalidArgument,
		"tag=" + asmr + "&tag_match=most": works.CodeInvalidArgument,
	} {
		response := list(query)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, response.Code)
			continue
		}
		if got, _ := errorEnvelope(t, response); got != code {
			t.Errorf("%s: code = %q, want %q", query, got, code)
		}
	}
}
