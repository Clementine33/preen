package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works"
)

type randomPick struct {
	RJ    string `json:"rj"`
	Index int    `json:"index"`
}

func getRandom(t *testing.T, engine http.Handler, query string) randomPick {
	t.Helper()
	w := doRequest(t, engine, http.MethodGet, "/api/v1/works/random"+query, nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET random%s: status = %d (body=%s)", query, w.Code, w.Body.String())
	}
	var pick randomPick
	if err := json.Unmarshal(w.Body.Bytes(), &pick); err != nil {
		t.Fatal(err)
	}
	return pick
}

// randomSeen draws repeatedly and returns every distinct work chosen.
func randomSeen(t *testing.T, engine http.Handler, query string) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	for range 40 {
		seen[getRandom(t, engine, query).RJ] = true
	}
	return seen
}

// TestRandomWorkPrefersUnheardAndAvoidsExclude covers the selection rules:
// unheard before listened/finished, the others as the fallback, the exclude
// work only when nothing else is left, and index as the listing position.
func TestRandomWorkPrefersUnheardAndAvoidsExclude(t *testing.T) {
	engine := newTestServer(t, func(d *httpapi.Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for _, id := range []string{"RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004", "RJ00000005"} {
			seedHTTPWork(t, fixture, id, "keep")
		}
		seedHTTPWork(t, fixture, "RJ00000006", "other")
		execAll(t, fixture,
			"UPDATE works SET release_date = '2020-01-0' || substr(id, 10) WHERE id <> 'RJ00000006'")
	})
	listen := func(rj string, finished bool) {
		body := snapshot("c", 1, 1)
		body["finished"] = finished
		putListening(t, engine, rj, body, http.StatusOK)
	}
	const scope = "?q=keep&sort=release_date&direction=desc"

	// 1 listening, 2 finished; 3, 4, 5 unheard.
	listen("RJ00000001", false)
	listen("RJ00000002", true)
	if seen := randomSeen(t, engine, scope); len(seen) != 3 || seen["RJ00000001"] || seen["RJ00000002"] {
		t.Errorf("with unheard works left, picked %v", seen)
	}
	// The exclude work is skipped while another unheard one exists; the code is
	// normalised from its 6-digit, lower-case spelling.
	if seen := randomSeen(t, engine, scope+"&exclude=rj000003"); len(seen) != 2 || seen["RJ00000003"] {
		t.Errorf("exclude=rj000003 picked %v", seen)
	}

	// index is the position in the ordinary listing under the same parameters:
	// release_date desc puts RJ00000005 first.
	want := map[string]int{"RJ00000005": 0, "RJ00000004": 1, "RJ00000003": 2, "RJ00000002": 3, "RJ00000001": 4}
	for range 20 {
		pick := getRandom(t, engine, scope)
		if pick.Index != want[pick.RJ] {
			t.Fatalf("pick %+v, want index %d", pick, want[pick.RJ])
		}
	}

	// The only unheard work is the excluded one: a heard work is returned
	// instead; as the sole candidate it is returned despite the exclude.
	listen("RJ00000004", true)
	listen("RJ00000005", false)
	if seen := randomSeen(t, engine, scope+"&exclude=RJ00000003"); len(seen) == 0 || seen["RJ00000003"] {
		t.Errorf("exclude was the only unheard work, picked %v", seen)
	}
	if seen := randomSeen(t, engine, "?q=RJ00000003&exclude=RJ00000003"); len(seen) != 1 || !seen["RJ00000003"] {
		t.Errorf("sole candidate equal to exclude, picked %v", seen)
	}

	// Nothing unheard left: any listened or finished work is a candidate.
	listen("RJ00000003", true)
	if seen := randomSeen(t, engine, scope); len(seen) < 2 {
		t.Errorf("fallback picked %v, want a spread over listened works", seen)
	}
}

func TestRandomWorkRefusals(t *testing.T) {
	engine := newTestServer(t, wireWorks)
	for _, query := range []string{"?limit=10", "?page=2", "?exclude=RJ1657200", "?exclude=abc", "?bogus=1"} {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/works/random"+query, nil, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, w.Code)
		} else if code, _ := errorEnvelope(t, w); code != works.CodeInvalidArgument {
			t.Errorf("%s: code = %q, want %q", query, code, works.CodeInvalidArgument)
		}
	}
	if w := doRequest(t, engine, http.MethodGet, "/api/v1/works/random?limit=5", nil, nil); w.Code == http.StatusOK {
		t.Fatal("limit must be refused")
	} else if _, msg := errorEnvelope(t, w); msg != "unknown query parameter: limit" {
		t.Errorf("message = %q", msg)
	}

	w := doRequest(t, engine, http.MethodGet, "/api/v1/works/random", nil, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("empty listing: status = %d, want 404", w.Code)
	}
	if code, _ := errorEnvelope(t, w); code != works.CodeWorkNotFound {
		t.Errorf("code = %q, want %q", code, works.CodeWorkNotFound)
	}
}
