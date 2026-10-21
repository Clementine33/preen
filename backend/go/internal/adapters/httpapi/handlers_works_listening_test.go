package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

type listeningRow struct {
	Rendition   string   `json:"rendition"`
	TrackKey    *string  `json:"track_key"`
	PositionSec *float64 `json:"position_sec"`
	TrackIndex  *int     `json:"track_index"`
	TrackCount  *int     `json:"track_count"`
	FinishedAt  *string  `json:"finished_at"`
}

func putListening(t *testing.T, engine http.Handler, rj string, body map[string]any, want int) listeningRow {
	t.Helper()
	w := doRequest(t, engine, http.MethodPut, "/api/v1/works/"+rj+"/listening", body, nil)
	if w.Code != want {
		t.Fatalf("PUT status = %d, want %d (body=%s)", w.Code, want, w.Body.String())
	}
	var row listeningRow
	if want == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
	}
	return row
}

type listeningSummary struct {
	State      string  `json:"state"`
	TrackIndex *int    `json:"track_index"`
	TrackCount *int    `json:"track_count"`
	UpdatedAt  *string `json:"updated_at"`
}

func snapshot(client string, seq int, position float64) map[string]any {
	return map[string]any{
		"rendition": "", "track_key": "a/one", "position_sec": position,
		"track_index": 1, "track_count": 3, "client": client, "seq": seq,
	}
}

func detailListening(t *testing.T, engine http.Handler, rj string) []listeningRow {
	t.Helper()
	w := doRequest(t, engine, http.MethodGet, "/api/v1/works/"+rj, nil, nil)
	var detail struct {
		Listening []listeningRow `json:"listening"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	return detail.Listening
}

func TestListeningPutUpsertsAndGuardsBySeq(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})
	putListening(t, engine, "RJ00000001", snapshot("a", 1, 10), http.StatusOK)
	row := putListening(t, engine, "RJ00000001", snapshot("a", 2, 20), http.StatusOK)
	if row.PositionSec == nil || *row.PositionSec != 20 {
		t.Fatalf("upsert position = %v, want 20", row.PositionSec)
	}
	// Same client, seq not greater: dropped, the stored row is answered.
	row = putListening(t, engine, "RJ00000001", snapshot("a", 2, 99), http.StatusOK)
	if *row.PositionSec != 20 {
		t.Errorf("stale same-client write applied: position = %v", *row.PositionSec)
	}
	// Another client always applies, whatever its seq.
	row = putListening(t, engine, "RJ00000001", snapshot("b", 1, 30), http.StatusOK)
	if *row.PositionSec != 30 {
		t.Errorf("other client's write dropped: position = %v", *row.PositionSec)
	}
	if rows := detailListening(t, engine, "RJ00000001"); len(rows) != 1 {
		t.Errorf("detail listening rows = %d, want one per rendition", len(rows))
	}
}

func TestListeningFinishedClearsPositionAndSurvivesLaterPosition(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})
	putListening(t, engine, "RJ00000001", snapshot("a", 1, 10), http.StatusOK)
	done := snapshot("a", 2, 0)
	done["finished"] = true
	row := putListening(t, engine, "RJ00000001", done, http.StatusOK)
	if row.FinishedAt == nil || row.TrackKey != nil || row.PositionSec != nil ||
		row.TrackIndex != nil || row.TrackCount != nil {
		t.Fatalf("finished row = %+v, want finished_at set and the position cleared", row)
	}
	finishedAt := *row.FinishedAt
	row = putListening(t, engine, "RJ00000001", snapshot("a", 3, 5), http.StatusOK)
	if row.FinishedAt == nil || *row.FinishedAt != finishedAt || row.PositionSec == nil || *row.PositionSec != 5 {
		t.Errorf("later position row = %+v, want finished_at kept and the new position", row)
	}
}

func TestListeningDeleteClearsEveryRendition(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})
	putListening(t, engine, "RJ00000001", snapshot("a", 1, 10), http.StatusOK)
	other := snapshot("a", 2, 10)
	other["rendition"] = "mp3"
	putListening(t, engine, "RJ00000001", other, http.StatusOK)
	if rows := detailListening(t, engine, "RJ00000001"); len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	w := doRequest(t, engine, http.MethodDelete, "/api/v1/works/RJ00000001/listening", nil, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", w.Code)
	}
	if rows := detailListening(t, engine, "RJ00000001"); len(rows) != 0 {
		t.Errorf("rows after DELETE = %d, want 0", len(rows))
	}
}

func TestListeningRefusals(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})
	for _, mutate := range map[string]func(map[string]any){
		"empty track_key": func(b map[string]any) { b["track_key"] = "" },
		"negative second": func(b map[string]any) { b["position_sec"] = -1 },
		"index at count":  func(b map[string]any) { b["track_index"] = 3 },
		"negative index":  func(b map[string]any) { b["track_index"] = -1 },
		"empty client":    func(b map[string]any) { b["client"] = "" },
	} {
		body := snapshot("a", 1, 1)
		mutate(body)
		putListening(t, engine, "RJ00000001", body, http.StatusBadRequest)
	}
	putListening(t, engine, "RJ00000009", snapshot("a", 1, 1), http.StatusNotFound)

	// Declaring a child creates a placeholder, which holds no listening.
	w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/metadata", map[string]any{
		"relations": map[string]any{
			"source": "dlsite", "fetched_at": stamp, "role": "parent", "child_rjs": []string{"RJ00000009"},
		},
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("import status = %d (body=%s)", w.Code, w.Body.String())
	}
	putListening(t, engine, "RJ00000009", snapshot("a", 1, 1), http.StatusConflict)
}

func TestListeningSummaryKeepsAnotherRenditionsPositionOnAFinishedWork(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "RJ00000001")
	})
	done := snapshot("a", 1, 0)
	done["rendition"], done["finished"] = "SE有", true
	putListening(t, engine, "RJ00000001", done, http.StatusOK)
	putListening(t, engine, "RJ00000001", snapshot("a", 2, 10), http.StatusOK)

	w := doRequest(t, engine, http.MethodGet, "/api/v1/works", nil, nil)
	var out struct {
		Works []struct {
			Listening listeningSummary `json:"listening"`
		} `json:"works"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Works) != 1 {
		t.Fatalf("listing = %s (%v)", w.Body.String(), err)
	}
	if got := out.Works[0].Listening; got.State != "finished" || got.TrackIndex == nil || *got.TrackIndex != 1 ||
		got.TrackCount == nil || *got.TrackCount != 3 {
		t.Errorf("summary = %+v, want finished with the other Rendition's track 1/3", got)
	}
}

func TestListeningSummaryFilterAndSort(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for _, rj := range []string{"RJ00000001", "RJ00000002", "RJ00000003", "RJ00000004"} {
			seedHTTPWork(t, fixture, rj, rj)
		}
	})
	// 2 listened first, then 3 finished, then 1 listened last; 4 never.
	putListening(t, engine, "RJ00000002", snapshot("a", 1, 10), http.StatusOK)
	done := snapshot("a", 1, 0)
	done["finished"] = true
	putListening(t, engine, "RJ00000003", done, http.StatusOK)
	putListening(t, engine, "RJ00000001", snapshot("a", 1, 10), http.StatusOK)

	list := func(query string) (rj []string, items map[string]listeningSummary) {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/works?"+query, nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d (body=%s)", query, w.Code, w.Body.String())
		}
		var out struct {
			Works []struct {
				RJ        string           `json:"rj"`
				Listening listeningSummary `json:"listening"`
			} `json:"works"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		items = map[string]listeningSummary{}
		for _, work := range out.Works {
			rj = append(rj, work.RJ)
			items[work.RJ] = work.Listening
		}
		return rj, items
	}

	_, items := list("")
	if got := items["RJ00000001"]; got.State != "listening" || *got.TrackIndex != 1 || *got.TrackCount != 3 ||
		got.UpdatedAt == nil {
		t.Errorf("listening summary = %+v", got)
	}
	if got := items["RJ00000003"]; got.State != "finished" || got.TrackIndex != nil || got.UpdatedAt != nil {
		t.Errorf("finished summary = %+v", got)
	}
	if got := items["RJ00000004"]; got.State != "none" || got.TrackIndex != nil {
		t.Errorf("none summary = %+v", got)
	}

	for query, want := range map[string]string{
		"listening=listening":             "RJ00000001,RJ00000002",
		"listening=finished":              "RJ00000003",
		"listening=none":                  "RJ00000004",
		"listening=finished,none":         "RJ00000003,RJ00000004",
		"sort=listened_at&direction=desc": "RJ00000001,RJ00000003,RJ00000002,RJ00000004",
		"sort=listened_at&direction=asc":  "RJ00000002,RJ00000003,RJ00000001,RJ00000004",
	} {
		rj, _ := list(query)
		if got := strings.Join(rj, ","); got != want {
			t.Errorf("%s: got %s, want %s", query, got, want)
		}
	}
	w := doRequest(t, engine, http.MethodGet, "/api/v1/works?listening=bogus", nil, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bogus listening state: status = %d, want 400", w.Code)
	}
}
