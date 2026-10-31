package httpapi //nolint:testpackage // white-box tests exercise unexported internals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/inventory"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"

	"github.com/Clementine33/preen/backend/internal/adapters/media"

	"github.com/Clementine33/preen/backend/internal/adapters/ffmpeg"
	appconfig "github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/workset"
)

const testToken = "test-token"

func newWorksetServer(t *testing.T, guards ...workset.EnqueueGuard) (http.Handler, *sqlite.Repository) {
	t.Helper()
	tmp := t.TempDir()
	repo, err := sqlite.NewRepository(tmp + "/test.db")
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	cfg := `{"prune":{"no_effect_tags":["SEなし"]}}`
	if err := os.WriteFile(filepath.Join(tmp, "config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	var guard workset.EnqueueGuard
	if len(guards) > 0 {
		guard = guards[0]
	}
	svc := workset.NewService(repo, 1, 1, []workset.Task{
		conversion.New(
			repo,
			appconfig.NewReader(tmp),
			func() execute.Encoder { return ffmpeg.New(ffmpeg.ToolsConfig{}) },
			media.ProbeBitrate,
			filesystem.Recover,
			filesystem.IsResolvedWithinRoot,
			conversion.ExecutionIO{
				Walk: func(ctx context.Context, roots []string, base string, emit func(string, inventory.DirEntry) error) error {
					return filesystem.WalkSelectedEntriesParallel(ctx, roots, base, 32, emit)
				},
				Moves: filesystem.ConversionMoves{},
				Files: filesystem.ConversionHandles{},
			},
		),
	}, nil, guard)
	handler := NewServer(Dependencies{
		Library:        library.NewService(repo, nil, repo, filesystem.ResolveMember),
		Catalog:        conversion.NewCatalog(repo, appconfig.NewReader(tmp)),
		Token:          testToken,
		CORSOrigins:    []string{"http://localhost:5173"},
		WorksetService: svc,
	})
	return handler, repo
}

func req(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return reqWithIfMatch(t, h, method, path, token, body, "")
}

func reqWithIfMatch(
	t *testing.T,
	h http.Handler,
	method, path, token string,
	body any,
	ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rdr)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if ifMatch != "" {
		r.Header.Set("If-Match", ifMatch)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func seedLibrary(t *testing.T, repo *sqlite.Repository) string {
	t.Helper()
	_, _ = repo.DB().
		Exec(`INSERT INTO libraries (id, name, root_path, root_path_key, created_at, updated_at) VALUES ('lib-1', 'Onsei', '/music', '/music', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	return "lib-1"
}

// seedMember records one library member in the scanned inventory — the
// directory row and an audio row beneath it — which is what a record's scope
// is resolved against.
func seedMember(t *testing.T, repo *sqlite.Repository, rel string) {
	t.Helper()
	inventoryfixture.SeedInventory(t, repo,
		inventoryfixture.Row{Path: "/music/albumA", RootPath: "/music", IsDir: true, IsWorkRoot: true},
		inventoryfixture.Row{Path: "/music/albumA/01.flac", RootPath: "/music", Size: 1024, Format: "flac"},
	)
}

// albumADirID is the directory id the overview lists for seedMember's folder.
func albumADirID(libID string) string {
	return library.DirID(libID, "/music", "albumA")
}

// createRecord creates the current conversion record of a library through the
// API and returns its id, failing the test when the creation is refused.
func createRecord(t *testing.T, h http.Handler, libID, key string) string {
	t.Helper()
	w := reqWithIdempotency(t, h, http.MethodPut, recordsPath(libID), testToken, map[string]any{
		"selection": map[string]any{"all": true},
	}, key)
	if w.Code != http.StatusCreated {
		t.Fatalf("create record: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Workset struct {
			WorksetID string `json:"workset_id"`
		} `json:"workset"`
		Created  bool `json:"created"`
		Recorded int  `json:"recorded"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	if !created.Created || created.Workset.WorksetID == "" || created.Recorded != 1 {
		t.Fatalf("create record resp: %+v", created)
	}
	return created.Workset.WorksetID
}

// recordsPath is the current-record route of one library's conversion.
func recordsPath(libID string) string {
	return "/api/v1/libraries/" + libID + "/operations/conversion/current"
}

// draftDocumentFixture is a complete operation draft document.
func draftDocumentFixture() map[string]any {
	profile := map[string]any{
		"lossless": map[string]any{"codec": "wav"},
		"encoded":  map[string]any{"codec": "mp3", "quality": map[string]any{"kind": "bitrate", "bitrate": 320}},
	}
	return map[string]any{
		"schema_version": 1,
		"mode":           "available_sources",
		"no_effect_tags": []string{"SEなし"},
		"matched":        profile,
		"unmatched":      profile,
		"members":        []any{},
	}
}

// commonOf is a draft fixture's common section: everything but its schema
// version and member records.
func commonOf(doc map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range doc {
		if key != "schema_version" && key != "members" {
			out[key] = value
		}
	}
	return out
}

//nolint:cyclop,gocognit,gocyclo,funlen // one HTTP lifecycle walk covering every operation route
func TestWorksetHTTPLifecycle(t *testing.T) {
	h, repo := newWorksetServer(t)
	libID := seedLibrary(t, repo)
	seedMember(t, repo, "albumA")

	// The library has no record yet; the current-record read says so without
	// making it an error.
	var none struct {
		Workset *json.RawMessage `json:"workset"`
	}
	empty := req(t, h, http.MethodGet, recordsPath(libID), testToken, nil)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty current record status = %d", empty.Code)
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &none); err != nil || none.Workset != nil {
		t.Fatalf("empty current record body = %s (err=%v)", empty.Body.String(), err)
	}

	// PUT create: the record of this (library, operation) pair, with the scope
	// it actually recorded.
	createBody := map[string]any{"selection": map[string]any{"dir_ids": []string{albumADirID(libID), "unknown"}}}
	w := reqWithIdempotency(t, h, http.MethodPut, recordsPath(libID), testToken, createBody, "create-http-1")
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", w.Code, w.Body.String())
	}
	var createResp struct {
		Workset struct {
			WorksetID string `json:"workset_id"`
			Version   int    `json:"version"`
		} `json:"workset"`
		Created  bool `json:"created"`
		Recorded int  `json:"recorded"`
		Skipped  []struct {
			DirID  string `json:"dir_id"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createResp.Workset.WorksetID == "" || !createResp.Created || createResp.Recorded != 1 {
		t.Fatalf("create resp: %+v", createResp)
	}
	if len(createResp.Skipped) != 1 || createResp.Skipped[0].DirID != "unknown" ||
		createResp.Skipped[0].Reason != "unknown_dir" {
		t.Fatalf("skipped = %+v, want the unknown id", createResp.Skipped)
	}
	// A retried create answers with the same record instead of a second one.
	replay := reqWithIdempotency(t, h, http.MethodPut, recordsPath(libID), testToken, createBody, "create-http-1")
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d, body=%s", replay.Code, replay.Body.String())
	}
	var replayResp struct {
		Workset struct {
			WorksetID string `json:"workset_id"`
		} `json:"workset"`
		Created bool `json:"created"`
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayResp); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if replayResp.Created || replayResp.Workset.WorksetID != createResp.Workset.WorksetID {
		t.Fatalf("replay resp: %+v", replayResp)
	}
	// The current-record read answers the same record.
	current := req(t, h, http.MethodGet, recordsPath(libID), testToken, nil)
	if current.Code != http.StatusOK || !strings.Contains(current.Body.String(), createResp.Workset.WorksetID) {
		t.Fatalf("current record = %d %s", current.Code, current.Body.String())
	}
	// The creation carries no title: the caller is not asked to name a record.
	if strings.Contains(w.Body.String(), `"title":""`) {
		t.Fatalf("a record without a title must still be created with a usable one: %s", w.Body.String())
	}
	wsPath := "/api/v1/worksets/" + createResp.Workset.WorksetID
	opPath := wsPath + "/operations/conversion"

	// The workset view has no operations; planning state is operation-scoped.
	w = req(t, h, http.MethodGet, wsPath, testToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("detail status = %d", w.Code)
	}
	var detail struct {
		Members []map[string]any `json:"members"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Members) != 1 {
		t.Fatalf("detail members: %+v", detail.Members)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &keys); err != nil {
		t.Fatalf("decode detail keys: %v", err)
	}
	if _, has := keys["operations"]; has {
		t.Fatalf("workset view must not carry operations: %s", w.Body.String())
	}
	if _, leaked := detail.Members[0]["state"]; leaked {
		t.Fatal("member coverage is operation state, not member state")
	}

	// GET operation, draft.
	if got := req(t, h, http.MethodGet, opPath, testToken, nil); got.Code != http.StatusOK {
		t.Fatalf("operation status = %d body=%s", got.Code, got.Body.String())
	}
	w = req(t, h, http.MethodGet, opPath+"/draft", testToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("draft status = %d body=%s", w.Code, w.Body.String())
	}
	var draft struct {
		Version int `json:"version"`
		Task    struct {
			Kind          string `json:"kind"`
			SchemaVersion int    `json:"schema_version"`
			Payload       struct {
				SchemaVersion int      `json:"schema_version"`
				Mode          string   `json:"mode"`
				NoEffectTags  []string `json:"no_effect_tags"`
				Members       []any    `json:"members"`
			} `json:"payload"`
		} `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if draft.Version != 1 || draft.Task.Kind != "conversion" || draft.Task.SchemaVersion != 1 ||
		draft.Task.Payload.SchemaVersion != 1 || draft.Task.Payload.Mode != "available_sources" {
		t.Fatalf("draft: %+v", draft)
	}
	if len(draft.Task.Payload.NoEffectTags) != 1 || len(draft.Task.Payload.Members) != 0 {
		t.Fatalf("seeded draft must be sparse: %+v", draft.Task.Payload)
	}

	// The common settings are written through the Task settings endpoint only.
	saveBody := draftDocumentFixture()
	if got := reqWithIfMatch(
		t,
		h,
		http.MethodPut,
		opPath+"/draft",
		testToken,
		saveBody,
		"1",
	); got.Code != http.StatusConflict ||
		!strings.Contains(got.Body.String(), "COMMON_SETTINGS_MISMATCH") {
		t.Fatalf("draft with different common settings = %d %s", got.Code, got.Body.String())
	}
	withFlag := draftDocumentFixture()
	withFlag["apply_common_settings"] = true
	if got := reqWithIfMatch(
		t,
		h,
		http.MethodPut,
		opPath+"/draft",
		testToken,
		withFlag,
		"1",
	); got.Code != http.StatusBadRequest {
		t.Fatalf("the retired apply flag must be rejected, got %d", got.Code)
	}
	settingsPath := "/api/v1/tasks/conversion/settings"
	seed := req(t, h, http.MethodGet, settingsPath, testToken, nil)
	etag := seed.Header().Get("ETag")
	if seed.Code != http.StatusOK || etag == "" {
		t.Fatalf("settings read = %d etag=%q body=%s", seed.Code, etag, seed.Body.String())
	}
	settingsBody := map[string]any{"settings": commonOf(saveBody)}
	if got := req(t, h, http.MethodPut, settingsPath, testToken, settingsBody); got.Code != http.StatusBadRequest {
		t.Fatalf("settings save without If-Match = %d, want 400", got.Code)
	}
	if got := reqWithIfMatch(
		t,
		h,
		http.MethodPut,
		settingsPath,
		testToken,
		settingsBody,
		`"nope"`,
	); got.Code != http.StatusConflict {
		t.Fatalf("settings save with a stale ETag = %d, want 409", got.Code)
	}
	if got := reqWithIfMatch(
		t,
		h,
		http.MethodPut,
		"/api/v1/tasks/nope/settings",
		testToken,
		settingsBody,
		etag,
	); got.Code != http.StatusNotFound &&
		got.Code != http.StatusBadRequest {
		t.Fatalf("unknown task = %d, want 4xx", got.Code)
	}
	put := reqWithIfMatch(t, h, http.MethodPut, settingsPath, testToken, settingsBody, etag)
	if put.Code != http.StatusOK || put.Header().Get("ETag") == "" || put.Header().Get("ETag") == etag {
		t.Fatalf("settings save = %d etag=%q body=%s", put.Code, put.Header().Get("ETag"), put.Body.String())
	}

	// PUT draft requires If-Match on the operation version.
	noMatch := req(t, h, http.MethodPut, opPath+"/draft", testToken, saveBody)
	if noMatch.Code != http.StatusBadRequest {
		t.Fatalf("draft save without If-Match = %d, want 400", noMatch.Code)
	}
	saved := reqWithIfMatch(t, h, http.MethodPut, opPath+"/draft", testToken, saveBody, "2")
	if saved.Code != http.StatusOK {
		t.Fatalf("put draft status = %d, body=%s", saved.Code, saved.Body.String())
	}
	var savedOp struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(saved.Body.Bytes(), &savedOp); err != nil {
		t.Fatalf("decode saved operation: %v", err)
	}
	if savedOp.Version != 3 {
		t.Fatalf("operation version after save = %d, want 3", savedOp.Version)
	}
	if common, err := repo.GetCommonSettings("conversion"); err != nil || common == nil {
		t.Fatalf("the settings save did not store a value: %+v err=%v", common, err)
	}
	// A stale If-Match on the operation version conflicts.
	stale := reqWithIfMatch(t, h, http.MethodPut, opPath+"/draft", testToken, saveBody, "2")
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale draft save = %d, want 409", stale.Code)
	}

	// POST revisions requires both Idempotency-Key and If-Match.
	noKey := reqWithIfMatch(t, h, http.MethodPost, opPath+"/revisions", testToken, nil, "3")
	if noKey.Code != http.StatusBadRequest {
		t.Fatalf("revisions without key status = %d, want 400", noKey.Code)
	}
	noVersion := reqWithIdempotency(t, h, http.MethodPost, opPath+"/revisions", testToken, nil, "gen-http-1")
	if noVersion.Code != http.StatusBadRequest {
		t.Fatalf("revisions without If-Match status = %d, want 400", noVersion.Code)
	}
	started := reqWithIdempotencyAndIfMatch(
		t,
		h,
		http.MethodPost,
		opPath+"/revisions",
		testToken,
		nil,
		"gen-http-1",
		"3",
	)
	if started.Code != http.StatusAccepted {
		t.Fatalf("start generation status = %d, body=%s", started.Code, started.Body.String())
	}
	var startedBody struct {
		Created    bool `json:"created"`
		Generation struct {
			GenerationID  string `json:"generation_id"`
			OperationType string `json:"operation_type"`
		} `json:"generation"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &startedBody); err != nil {
		t.Fatalf("decode generation: %v", err)
	}
	if !startedBody.Created || startedBody.Generation.OperationType != "conversion" {
		t.Fatalf("generation body: %+v", startedBody)
	}

	// The session is addressable through its operation only.
	genPath := opPath + "/planning-sessions/" + startedBody.Generation.GenerationID
	if got := req(t, h, http.MethodGet, genPath, testToken, nil); got.Code != http.StatusOK {
		t.Fatalf("session detail = %d body=%s", got.Code, got.Body.String())
	}
	wrongOp := req(t, h, http.MethodGet,
		wsPath+"/operations/rename/planning-sessions/"+startedBody.Generation.GenerationID, testToken, nil)
	if wrongOp.Code != http.StatusNotFound {
		t.Fatalf("session through another operation = %d, want 404", wrongOp.Code)
	}
	if got := req(t, h, http.MethodGet, wsPath+"/operations/rename", testToken, nil); got.Code != http.StatusNotFound {
		t.Fatalf("unknown operation = %d, want 404", got.Code)
	}

	// Cancel the queued session; a canceled session publishes nothing.
	if got := req(t, h, http.MethodPost, genPath+"/cancel", testToken, nil); got.Code != http.StatusOK {
		t.Fatalf("cancel status = %d body=%s", got.Code, got.Body.String())
	}
	// There is no route to a record's revision history: a record keeps the
	// current plan only, and the old list route is gone (ADR 0001).
	if got := req(t, h, http.MethodGet, opPath+"/revisions", testToken, nil); got.Code == http.StatusOK {
		t.Fatalf("a record's revision history must not be readable: %s", got.Body.String())
	}

	// Renaming uses the workset metadata version only.
	renamed := reqWithIfMatch(t, h, http.MethodPatch, wsPath, testToken,
		map[string]any{"title": "改名"}, strconv.Itoa(createResp.Workset.Version))
	if renamed.Code != http.StatusOK {
		t.Fatalf("rename status = %d body=%s", renamed.Code, renamed.Body.String())
	}
	afterRename := reqWithIfMatch(t, h, http.MethodPut, opPath+"/draft", testToken, saveBody, "3")
	if afterRename.Code != http.StatusOK {
		t.Fatalf(
			"rename must not invalidate the operation version: %d body=%s",
			afterRename.Code,
			afterRename.Body.String(),
		)
	}
}

// reqWithIdempotency sends a request with an Idempotency-Key header.
func reqWithIdempotency(
	t *testing.T,
	h http.Handler,
	method, path, token string,
	body any,
	idemKey string,
) *httptest.ResponseRecorder {
	t.Helper()
	return reqWithIdempotencyAndIfMatch(t, h, method, path, token, body, idemKey, "")
}

func reqWithIdempotencyAndIfMatch(
	t *testing.T,
	h http.Handler,
	method, path, token string,
	body any,
	idemKey, ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rdr)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if idemKey != "" {
		r.Header.Set("Idempotency-Key", idemKey)
	}
	if ifMatch != "" {
		r.Header.Set("If-Match", ifMatch)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestWorksetAuthRequired(t *testing.T) {
	h, repo := newWorksetServer(t)
	seedLibrary(t, repo)
	seedMember(t, repo, "albumA")
	w := reqWithIdempotency(t, h, http.MethodPut, recordsPath("lib-1"), "",
		map[string]any{"selection": map[string]any{"all": true}}, "create-auth")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth status = %d, want 401", w.Code)
	}
}

func TestPolicySlotRoutesAreAbsent(t *testing.T) {
	h, _ := newWorksetServer(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/policy-slots"},
		{http.MethodPut, "/api/v1/policy-slots/1"},
	} {
		w := req(t, h, route.method, route.path, testToken, nil)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"code":"NOT_FOUND"`) {
			t.Fatalf("removed route %s %s = %d %s", route.method, route.path, w.Code, w.Body.String())
		}
	}
}

func TestClassifierTagsHTTPAPI(t *testing.T) {
	h, _ := newWorksetServer(t)

	// 1. Initial list: has default tags from temp config, 0 custom tags
	w := req(t, h, http.MethodGet, "/api/v1/classifier-tags", testToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list tags status = %d", w.Code)
	}
	var res struct {
		DefaultTags []string `json:"default_no_effect_tags"`
		CustomTags  []struct {
			ID        int64  `json:"id"`
			Tag       string `json:"tag"`
			CreatedAt string `json:"created_at"`
		} `json:"custom_tags"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode tag library: %v", err)
	}
	if len(res.DefaultTags) == 0 || res.DefaultTags[0] != "SEなし" {
		t.Fatalf("expected default tags [SEなし], got %+v", res.DefaultTags)
	}
	if len(res.CustomTags) != 0 {
		t.Fatalf("expected 0 custom tags initially, got %d", len(res.CustomTags))
	}

	// 2. Add custom tag
	w = req(t, h, http.MethodPost, "/api/v1/classifier-tags", testToken, map[string]any{"tag": "  效果音なし  "})
	if w.Code != http.StatusCreated {
		t.Fatalf("create tag status = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID  int64  `json:"id"`
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created tag: %v", err)
	}
	if created.Tag != "效果音なし" || created.ID <= 0 {
		t.Fatalf("unexpected created tag: %+v", created)
	}

	// 3. Duplicate is idempotent
	w = req(t, h, http.MethodPost, "/api/v1/classifier-tags", testToken, map[string]any{"tag": "效果音なし"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create duplicate tag status = %d", w.Code)
	}

	// 4. Verify in list
	w = req(t, h, http.MethodGet, "/api/v1/classifier-tags", testToken, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode tag library 2: %v", err)
	}
	if len(res.CustomTags) != 1 || res.CustomTags[0].Tag != "效果音なし" {
		t.Fatalf("expected 1 custom tag, got %+v", res.CustomTags)
	}

	// 5. Delete custom tag
	delURL := fmt.Sprintf("/api/v1/classifier-tags/%d", created.ID)
	w = req(t, h, http.MethodDelete, delURL, testToken, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", w.Code, w.Body.String())
	}

	// 6. Verify list is empty
	w = req(t, h, http.MethodGet, "/api/v1/classifier-tags", testToken, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if len(res.CustomTags) != 0 {
		t.Fatalf("expected 0 custom tags after delete, got %d", len(res.CustomTags))
	}
}

func TestConversionTagWriteBoundaries(t *testing.T) {
	h, repo := newWorksetServer(t)
	libID := seedLibrary(t, repo)
	seedMember(t, repo, "albumA")
	id := createRecord(t, h, libID, "tags-write")
	path := "/api/v1/worksets/" + id + "/operations/conversion/draft"
	detail := req(t, h, http.MethodGet, "/api/v1/worksets/"+id, testToken, nil)
	var view struct {
		Members []struct {
			MemberID string `json:"member_id"`
		}
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Members) != 1 {
		t.Fatalf("members: %s", detail.Body.String())
	}
	for _, tc := range []struct {
		name     string
		key      string
		value    any
		override bool
	}{
		{"old key empty", "classifier_tags", []string{}, false},
		{"old key null", "classifier_tags", nil, false},
		{"old key case variant", "CLASSIFIER_TAGS", []string{"old"}, false},
		{"slash", "other_tags", []string{"wav/revert"}, false},
		{"backslash", "other_tags", []string{`wav\revert`}, false},
		{"excluded override slash", "other_tags", []string{"wav/revert"}, true},
		{"excluded override backslash", "other_tags", []string{`wav\revert`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := draftDocumentFixture()
			if tc.override {
				doc["members"] = []any{
					map[string]any{
						"member_id": view.Members[0].MemberID,
						"excluded":  true,
						"overrides": map[string]any{tc.key: tc.value},
					},
				}
			} else {
				doc[tc.key] = tc.value
			}
			w := reqWithIfMatch(t, h, http.MethodPut, path, testToken, doc, "1")
			if w.Code != http.StatusBadRequest {
				t.Fatalf("draft must reject malformed tags: %d %s", w.Code, w.Body.String())
			}
		})
	}
	doc := draftDocumentFixture()
	doc["no_effect_tags"] = []string{}
	doc["other_tags"] = []string{}
	seed := req(t, h, http.MethodGet, "/api/v1/tasks/conversion/settings", testToken, nil)
	saved := reqWithIfMatch(t, h, http.MethodPut, "/api/v1/tasks/conversion/settings", testToken,
		map[string]any{"settings": commonOf(doc)}, seed.Header().Get("ETag"))
	if saved.Code != http.StatusOK {
		t.Fatalf("empty no-effect draft must save: %d %s", saved.Code, saved.Body.String())
	}
}
