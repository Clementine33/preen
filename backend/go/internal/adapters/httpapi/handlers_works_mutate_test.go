package httpapi //nolint:testpackage // white-box tests drive the routes through the real router

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works"
)

// TestMetadataImportAppliesGroupsAndReportsSkips covers the submission contract:
// a group that supersedes what is stored is applied, one that is stale is
// reported as skipped rather than as a failure, and the response keeps the two
// apart.
func TestMetadataImportAppliesGroupsAndReportsSkips(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})

	body := map[string]any{
		"core": map[string]any{
			"source":     "dlsite",
			"fetched_at": stamp,
			"title":      "imported title",
			"nsfw":       true,
		},
		"statistics": map[string]any{"fetched_at": stamp, "dl_count": 12},
	}
	w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/metadata", body, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var result struct {
		Applied []struct {
			DataGroup string `json:"data_group"`
		} `json:"applied"`
		Ignored  []any `json:"ignored"`
		Warnings []any `json:"warnings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 2 {
		t.Errorf("applied = %+v, want both groups", result.Applied)
	}
	if len(result.Ignored) != 0 || len(result.Warnings) != 0 {
		t.Errorf("ignored=%v warnings=%v, want neither", result.Ignored, result.Warnings)
	}

	// The same fetch time again is stale, and saying so is not a failure.
	w = doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/metadata", map[string]any{
		"statistics": map[string]any{"fetched_at": stamp, "dl_count": 999},
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var second struct {
		Applied []any `json:"applied"`
		Ignored []struct {
			DataGroup string `json:"data_group"`
			Reason    string `json:"reason"`
		} `json:"ignored"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Applied) != 0 || len(second.Ignored) != 1 {
		t.Fatalf("applied=%v ignored=%v, want the stale group skipped", second.Applied, second.Ignored)
	}
	if second.Ignored[0].Reason != works.ReasonStaleFetchedAt {
		t.Errorf("reason = %q, want %q", second.Ignored[0].Reason, works.ReasonStaleFetchedAt)
	}

	// The stored figure is the first one, not the one that was skipped.
	w = doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ00000001", nil, nil)
	var detail struct {
		DLCount *int    `json:"dl_count"`
		Title   *string `json:"title"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.DLCount == nil || *detail.DLCount != 12 {
		t.Errorf("dl_count = %v, want the stored 12", detail.DLCount)
	}
	if detail.Title == nil || *detail.Title != "imported title" {
		t.Errorf("title = %v", detail.Title)
	}
}

// TestMetadataImportRefusalsAreWholeSubmission: every refusal is a 400 that
// writes nothing at all, so a submission never lands partially.
func TestMetadataImportRefusalsAreWholeSubmission(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})

	cases := []struct {
		name string
		path string
		body any
		code string
	}{
		{
			name: "an invalid work number in the path",
			path: "/api/v1/works/RJ01234/metadata",
			body: map[string]any{"statistics": map[string]any{"fetched_at": stamp}},
			code: works.CodeInvalidArgument,
		},
		{
			name: "an unknown field",
			path: "/api/v1/works/RJ00000001/metadata",
			body: map[string]any{"statistics": map[string]any{"fetched_at": stamp, "price": 100}},
			code: works.CodeInvalidArgument,
		},
		{
			name: "a malformed fetch time",
			path: "/api/v1/works/RJ00000001/metadata",
			body: map[string]any{"statistics": map[string]any{"fetched_at": "2026-09-27T10:00:00Z"}},
			code: works.CodeTimestampFormat,
		},
		{
			name: "a negative sale count",
			path: "/api/v1/works/RJ00000001/metadata",
			body: map[string]any{"statistics": map[string]any{"fetched_at": stamp, "dl_count": -5}},
			code: works.CodeInvalidArgument,
		},
		{
			name: "an unknown source",
			path: "/api/v1/works/RJ00000001/metadata",
			body: map[string]any{"core": map[string]any{"source": "somewhere", "fetched_at": stamp}},
			code: works.CodeInvalidArgument,
		},
		{
			name: "a work that is its own parent",
			path: "/api/v1/works/RJ00000001/metadata",
			body: map[string]any{"relations": map[string]any{
				"source": "dlsite", "fetched_at": stamp, "role": "child", "parent_rj": "RJ00000001",
			}},
			code: works.CodeRelationSelfReference,
		},
		{
			name: "an empty submission",
			path: "/api/v1/works/RJ00000001/metadata",
			body: map[string]any{},
			code: works.CodeInvalidArgument,
		},
		{
			name: "a work that does not exist",
			path: "/api/v1/works/RJ00000009/metadata",
			body: map[string]any{"statistics": map[string]any{"fetched_at": stamp}},
			code: works.CodeWorkNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(t, engine, http.MethodPut, tc.path, tc.body, nil)
			if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 400 or 404 (body=%s)", w.Code, w.Body.String())
			}
			code, _ := errorEnvelope(t, w)
			if code != tc.code {
				t.Errorf("code = %q, want %q", code, tc.code)
			}
		})
	}

	// Nothing the refused submissions mentioned was written.
	w := doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ00000001", nil, nil)
	var detail struct {
		DLCount *int `json:"dl_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.DLCount != nil {
		t.Errorf("dl_count = %v, want nothing written by the refused submissions", detail.DLCount)
	}
}

// TestUserDataRatingIsOneToTenThroughTheAPI covers the rating contract at the
// boundary the client sees: the range, the explicit clear, and the fields the
// request did not mention.
func TestUserDataRatingIsOneToTenThroughTheAPI(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})

	for _, value := range []any{0, 11, 7.5} {
		w := doRequest(t, engine, http.MethodPatch, "/api/v1/works/RJ00000001/user-data",
			map[string]any{"rating": value}, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("rating %v: status = %d, want 400 (body=%s)", value, w.Code, w.Body.String())
		}
	}

	w := doRequest(t, engine, http.MethodPatch, "/api/v1/works/RJ00000001/user-data",
		map[string]any{"rating": 8, "favorite": true, "note": "a note"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	// A request that mentions only the note leaves the rating and the flag.
	if noteOnly := doRequest(t, engine, http.MethodPatch, "/api/v1/works/RJ00000001/user-data",
		map[string]any{"note": "another note"}, nil); noteOnly.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", noteOnly.Code, noteOnly.Body.String())
	}
	// An explicit null clears the rating without touching the rest.
	w = doRequest(t, engine, http.MethodPatch, "/api/v1/works/RJ00000001/user-data",
		map[string]any{"rating": nil}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var data struct {
		Rating   *int   `json:"rating"`
		Favorite bool   `json:"favorite"`
		Note     string `json:"note"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Rating != nil {
		t.Errorf("rating = %v, want it cleared", data.Rating)
	}
	if !data.Favorite {
		t.Error("the favorite flag was cleared by a request that did not mention it")
	}
	if data.Note != "another note" {
		t.Errorf("note = %q, want the last one written", data.Note)
	}
}

// TestUserDataIsRefusedOnAPlaceholder: a work declared by a relation but never
// found on disk offers no side door into the library.
func TestUserDataIsRefusedOnAPlaceholder(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})
	// Declaring a child creates a placeholder for it.
	w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/metadata", map[string]any{
		"relations": map[string]any{
			"source": "dlsite", "fetched_at": stamp, "role": "parent", "child_rjs": []string{"RJ00000009"},
		},
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("import status = %d (body=%s)", w.Code, w.Body.String())
	}

	w = doRequest(t, engine, http.MethodPatch, "/api/v1/works/RJ00000009/user-data",
		map[string]any{"rating": 8}, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", w.Code, w.Body.String())
	}

	// And the placeholder is absent from the ordinary listing.
	list := doRequest(t, engine, http.MethodGet, "/api/v1/works", nil, nil)
	assertWorks(t, list, "RJ00000001")
}

// TestUserTagLifecycle: creating by name is idempotent, renaming onto a taken
// name is a conflict, and deleting a tag leaves the works alone.
func TestUserTagLifecycle(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the work")
	})

	created := doRequest(t, engine, http.MethodPost, "/api/v1/user-tags",
		map[string]any{"name": "favourite"}, nil)
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d (body=%s)", created.Code, created.Body.String())
	}
	var first userTagResponse
	if err := json.Unmarshal(created.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}

	again := doRequest(t, engine, http.MethodPost, "/api/v1/user-tags",
		map[string]any{"name": "favourite"}, nil)
	if again.Code != http.StatusOK {
		t.Fatalf("second create status = %d, want 200 (body=%s)", again.Code, again.Body.String())
	}
	var second userTagResponse
	if err := json.Unmarshal(again.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Errorf("creating the same name made a second tag: %s vs %s", first.ID, second.ID)
	}

	// A blank name is refused rather than stored.
	if w := doRequest(t, engine, http.MethodPost, "/api/v1/user-tags",
		map[string]any{"name": "   "}, nil); w.Code != http.StatusBadRequest {
		t.Errorf("blank name status = %d, want 400", w.Code)
	}

	// Attaching the tag, then replacing the set with nothing.
	w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/user-tags",
		map[string]any{"tag_ids": []string{first.ID}}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("attach status = %d (body=%s)", w.Code, w.Body.String())
	}
	if w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/user-tags",
		map[string]any{"tag_ids": []string{"no-such-tag"}}, nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown tag id status = %d, want 404", w.Code)
	}
	if w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/user-tags",
		map[string]any{"tag_ids": []string{}}, nil); w.Code != http.StatusOK {
		t.Errorf("clearing status = %d, want 200", w.Code)
	}

	// Renaming onto a taken name is a conflict.
	other := doRequest(t, engine, http.MethodPost, "/api/v1/user-tags",
		map[string]any{"name": "later"}, nil)
	var otherTag userTagResponse
	if err := json.Unmarshal(other.Body.Bytes(), &otherTag); err != nil {
		t.Fatal(err)
	}
	if w := doRequest(t, engine, http.MethodPatch, "/api/v1/user-tags/"+otherTag.ID,
		map[string]any{"name": "favourite"}, nil); w.Code != http.StatusConflict {
		t.Errorf("conflicting rename status = %d, want 409", w.Code)
	}

	if w := doRequest(
		t,
		engine,
		http.MethodDelete,
		"/api/v1/user-tags/"+first.ID,
		nil,
		nil,
	); w.Code != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", w.Code)
	}
	if w := doRequest(
		t,
		engine,
		http.MethodDelete,
		"/api/v1/user-tags/"+first.ID,
		nil,
		nil,
	); w.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", w.Code)
	}

	// Deleting a tag never removes the work.
	if w := doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ00000001", nil, nil); w.Code != http.StatusOK {
		t.Errorf("the work did not survive a tag deletion: %d", w.Code)
	}
}

// TestDeleteWorkRefusesWhileReferenced: the two things that hold a work in
// place, and the deletion that succeeds once neither is there.
func TestDeleteWorkRefusesWhileReferenced(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00000001", "the original")
		seedHTTPWork(t, fixture, "RJ00000002", "the translation")
	})

	// The translation names the original.
	if w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000002/metadata", map[string]any{
		"relations": map[string]any{
			"source": "dlsite", "fetched_at": stamp, "role": "child", "parent_rj": "RJ00000001",
		},
	}, nil); w.Code != http.StatusOK {
		t.Fatalf("import status = %d (body=%s)", w.Code, w.Body.String())
	}

	conflict := doRequest(t, engine, http.MethodDelete, "/api/v1/works/RJ00000001", nil, nil)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", conflict.Code, conflict.Body.String())
	}
	var envelope struct {
		Code    string   `json:"code"`
		Details []string `json:"details"`
	}
	if err := json.Unmarshal(conflict.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != works.CodeWorkInUse {
		t.Errorf("code = %q, want %q", envelope.Code, works.CodeWorkInUse)
	}
	if len(envelope.Details) == 0 {
		t.Error("the refusal named no reference")
	}

	// Deleting the translation is fine, and then the original is free.
	if w := doRequest(
		t,
		engine,
		http.MethodDelete,
		"/api/v1/works/RJ00000002",
		nil,
		nil,
	); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204 (body=%s)", w.Code, w.Body.String())
	}
	if w := doRequest(
		t,
		engine,
		http.MethodDelete,
		"/api/v1/works/RJ00000001",
		nil,
		nil,
	); w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204 (body=%s)", w.Code, w.Body.String())
	}
	if w := doRequest(t, engine, http.MethodGet, "/api/v1/works/RJ00000001", nil, nil); w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want the work to be gone", w.Code)
	}
}

// TestDetailAddressesTheCanonicalNumber: the short spelling and the canonical
// one name the same work, because they are the same number.
func TestDetailAddressesTheCanonicalNumber(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		seedHTTPWork(t, fixture, "RJ00123456", "the work")
	})

	for _, spelling := range []string{"RJ00123456", "RJ123456", "rj123456"} {
		w := doRequest(t, engine, http.MethodGet, "/api/v1/works/"+spelling, nil, nil)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (body=%s)", spelling, w.Code, w.Body.String())
			continue
		}
		var detail struct {
			RJ string `json:"rj"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		if detail.RJ != "RJ00123456" {
			t.Errorf("%s resolved to %s", spelling, detail.RJ)
		}
	}
}

func TestAgeCategoriesPersistAndFilterIndependently(t *testing.T) {
	ids := []string{"RJ00000001", "RJ00000002", "RJ00000003"}
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) {
		wireWorks(d, fixture)
		for _, id := range ids {
			seedHTTPWork(t, fixture, id, "work")
		}
		seedHTTPWork(t, fixture, "RJ00000004", "unreported age")
	})
	for i, id := range ids {
		age := i + 1
		w := doRequest(t, engine, http.MethodPut, "/api/v1/works/"+id+"/metadata", map[string]any{
			"core": map[string]any{"source": "dlsite", "fetched_at": stamp, "age_category": age},
		}, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("import: %d %s", w.Code, w.Body.String())
		}
		w = doRequest(t, engine, http.MethodGet, "/api/v1/works/"+id, nil, nil)
		var detail struct {
			AgeCategory *int  `json:"age_category"`
			NSFW        *bool `json:"nsfw"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		if detail.AgeCategory == nil || *detail.AgeCategory != age || detail.NSFW == nil || *detail.NSFW != (age == 3) {
			t.Fatalf("incorrect age: %s", w.Body.String())
		}
		w = doRequest(t, engine, http.MethodGet, "/api/v1/works?age_category="+strconv.Itoa(age), nil, nil)
		var page struct {
			Works []struct {
				RJ          string `json:"rj"`
				AgeCategory *int   `json:"age_category"`
			} `json:"works"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || len(page.Works) != 1 || page.Works[0].RJ != id ||
			page.Works[0].AgeCategory == nil || *page.Works[0].AgeCategory != age {
			t.Fatalf("incorrect age filter: %s", w.Body.String())
		}
	}
	for _, value := range []any{0, 4, 1.5, "bad"} {
		w := doRequest(t, engine, http.MethodPut, "/api/v1/works/RJ00000001/metadata", map[string]any{
			"core": map[string]any{"source": "dlsite", "fetched_at": stamp, "age_category": value},
		}, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid age accepted: %v", value)
		}
	}
	w := doRequest(t, engine, http.MethodGet, "/api/v1/works?age_category=4", nil, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid filter accepted: %s", w.Body.String())
	}
}

// TestPreferencesRoundTrip: the Format Preference reads empty before any save,
// is normalized on write, survives a read, and refuses unknown keys and blanks.
func TestPreferencesRoundTrip(t *testing.T) {
	engine := newTestServer(t, func(d *Dependencies, fixture *sqlite.Repository) { wireWorks(d, fixture) })
	read := func() string {
		return doRequest(t, engine, http.MethodGet, "/api/v1/preferences", nil, nil).Body.String()
	}

	if got := read(); got != `{"track_formats":[]}` {
		t.Fatalf("initial = %s", got)
	}
	w := doRequest(t, engine, http.MethodPut, "/api/v1/preferences",
		map[string]any{"track_formats": []string{" wav", "FLAC", "Wav", "mp3"}}, nil)
	if w.Code != http.StatusOK || w.Body.String() != `{"track_formats":["WAV","FLAC","MP3"]}` {
		t.Fatalf("put = %d %s", w.Code, w.Body.String())
	}
	if got := read(); got != `{"track_formats":["WAV","FLAC","MP3"]}` {
		t.Fatalf("read back = %s", got)
	}
	for name, body := range map[string]any{
		"unknown key": map[string]any{"track_formats": []string{"WAV"}, "x": 1},
		"blank":       map[string]any{"track_formats": []string{"WAV", "  "}},
		"missing":     map[string]any{},
	} {
		if w := doRequest(
			t,
			engine,
			http.MethodPut,
			"/api/v1/preferences",
			body,
			nil,
		); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, w.Code)
		}
	}
	if got := read(); got != `{"track_formats":["WAV","FLAC","MP3"]}` {
		t.Fatalf("rejected writes changed state: %s", got)
	}
}
