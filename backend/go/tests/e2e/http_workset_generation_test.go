package e2e //nolint:testpackage // white-box tests exercise unexported internals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHTTPWorksetGenerationLoop boots the real backend, creates and scans a
// library, creates its current conversion record from the scanned album
// folders, starts an async generation on the seeded balanced draft, polls the
// generation detail to completion, reviews the immutable plan, verifies
// unchanged-generation replay, then deletes the library and checks that its
// record went with it while the media stayed on disk.
//
//nolint:gocognit,gocyclo,cyclop,funlen // e2e generation loop
func TestHTTPWorksetGenerationLoop(t *testing.T) {
	binPath := buildBackendBinary(t)
	dataDir := t.TempDir()
	rootPath := filepath.Join(dataDir, "music")
	mustWriteFile(t, filepath.Join(rootPath, "RJ000001 albumA", "test1.mp3"), "dummy audio")
	mustWriteFile(t, filepath.Join(rootPath, "RJ000001 albumA", "test1.flac"), "dummy audio")
	mustWriteFile(t, filepath.Join(rootPath, "RJ000001 albumA", "test2.mp3"), "dummy audio")
	mustWriteFile(t, filepath.Join(rootPath, "RJ000001 albumA", "test2.flac"), "dummy audio")

	const token = "e2e-token"
	proc := startBackendBinary(t, binPath, dataDir, token)
	base := fmt.Sprintf("http://127.0.0.1:%d", proc.httpPort)
	client := &http.Client{Timeout: 120 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create + scan the library.
	libReq := map[string]string{"name": "E2E", "root_path": filepath.ToSlash(rootPath)}
	var lib struct{ ID string }
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodPost,
		"/api/v1/libraries",
		token,
		libReq,
		&lib,
	); code != http.StatusCreated {
		t.Fatalf("create lib: %d", code)
	}
	events := postScanSSE(t, client, ctx, base, lib.ID, token)
	if events["completed"] == nil {
		t.Fatal("scan SSE missing completed")
	}

	var dirs struct {
		Dirs []struct {
			RelPath string `json:"rel_path"`
			Path    string `json:"path"`
			DirID   string `json:"dir_id"`
		} `json:"dirs"`
	}
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		"/api/v1/libraries/"+lib.ID+"/dirs",
		token,
		nil,
		&dirs,
	); code != http.StatusOK ||
		len(dirs.Dirs) == 0 {
		t.Fatalf("dirs: code=%d n=%d", code, len(dirs.Dirs))
	}
	albumRel, albumDirID := "", ""
	for _, d := range dirs.Dirs {
		if d.RelPath == "RJ000001 albumA" {
			albumRel, albumDirID = d.RelPath, d.DirID
		}
	}
	if albumRel == "" || albumDirID == "" {
		t.Fatalf("RJ000001 albumA missing from the overview listing: %+v", dirs.Dirs)
	}

	// The identity the listing hands out opens that directory's tree: the
	// address carries no name, and the resolved path comes back with the tree.
	var tree struct {
		DirID      string `json:"dir_id"`
		MemberPath string `json:"member_path"`
	}
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		"/api/v1/libraries/"+lib.ID+"/tree?dir="+albumDirID,
		token,
		nil,
		&tree,
	); code != http.StatusOK || tree.MemberPath != "RJ000001 albumA" || tree.DirID != albumDirID {
		t.Fatalf("tree by identity: code=%d %+v (identity=%s)", code, tree, albumDirID)
	}

	// Create the library's current conversion record.
	wsReq := map[string]any{"selection": map[string]any{"dir_ids": []string{albumDirID}}}
	var wsResp struct {
		Workset struct {
			WorksetID string `json:"workset_id"`
			Version   int    `json:"version"`
		} `json:"workset"`
		Created bool `json:"created"`
	}
	if code := doJSONWithHeaders(
		t,
		client,
		ctx,
		base,
		http.MethodPut,
		"/api/v1/libraries/"+lib.ID+"/operations/conversion/current",
		token,
		map[string]string{"Idempotency-Key": "create-e2e-1"},
		wsReq,
		&wsResp,
	); code != http.StatusCreated {
		t.Fatalf("create record: %d", code)
	}
	wsID := wsResp.Workset.WorksetID
	if wsID == "" {
		t.Fatal("empty workset id")
	}

	// The record hands out the same identity for that directory: the conversion
	// entry addresses the shared file cache exactly as the overview does, and
	// that identity is what opens the member's tree.
	var record struct {
		Workset struct {
			Members []struct {
				RelPath string `json:"rel_path"`
				DirID   string `json:"dir_id"`
			} `json:"members"`
		} `json:"workset"`
	}
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		"/api/v1/libraries/"+lib.ID+"/operations/conversion/current",
		token,
		nil,
		&record,
	); code != http.StatusOK || len(record.Workset.Members) != 1 {
		t.Fatalf("current record: code=%d %+v", code, record)
	}
	member := record.Workset.Members[0]
	if member.DirID != albumDirID {
		t.Fatalf("record member identity = %q, listing identity = %q", member.DirID, albumDirID)
	}
	tree = struct {
		DirID      string `json:"dir_id"`
		MemberPath string `json:"member_path"`
	}{}
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		"/api/v1/libraries/"+lib.ID+"/tree?dir="+member.DirID,
		token,
		nil,
		&tree,
	); code != http.StatusOK || tree.MemberPath != "RJ000001 albumA" {
		t.Fatalf("tree by the record's identity: code=%d %+v (identity=%s)", code, tree, member.DirID)
	}

	// Opening the page refreshes the member, and that refresh must not unlist
	// it: the directory is still there, so its identity must keep resolving and
	// the overview must keep listing it.
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodPost,
		"/api/v1/libraries/"+lib.ID+"/tree/refresh?dir="+member.DirID,
		token,
		map[string]any{},
		&tree,
	); code != http.StatusOK || tree.MemberPath != "RJ000001 albumA" {
		t.Fatalf("refresh by identity: code=%d %+v (identity=%s)", code, tree, member.DirID)
	}
	var afterRefresh struct {
		Dirs []struct {
			RelPath string `json:"rel_path"`
			DirID   string `json:"dir_id"`
		} `json:"dirs"`
	}
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		"/api/v1/libraries/"+lib.ID+"/dirs",
		token,
		nil,
		&afterRefresh,
	); code != http.StatusOK || len(afterRefresh.Dirs) != 1 {
		t.Fatalf("listing after a member refresh: code=%d %+v", code, afterRefresh.Dirs)
	}
	if afterRefresh.Dirs[0].RelPath != "RJ000001 albumA" || afterRefresh.Dirs[0].DirID != member.DirID {
		t.Fatalf("the refreshed member came back as %+v, want RJ000001 albumA with identity %q",
			afterRefresh.Dirs[0], member.DirID)
	}

	wsPath := "/api/v1/worksets/" + wsID
	opPath := wsPath + "/operations/conversion"

	// Start generation on the seeded draft. Idempotency-Key and If-Match (the
	// operation version) are both required.
	if code := doJSONWithHeaders(
		t,
		client,
		ctx,
		base,
		http.MethodPost,
		opPath+"/revisions",
		token,
		map[string]string{"Idempotency-Key": "gen-key-1", "If-Match": "1"},
		nil,
		nil,
	); code != http.StatusAccepted {
		t.Fatalf("start generation: %d", code)
	}

	// Poll the workset detail until the active generation completes.
	var lastGenID string
	var genDetail struct {
		Status       string `json:"status"`
		RevisionID   string `json:"revision_id"`
		CompletedInt int    `json:"completed_roots"`
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		var opDetail struct {
			ActiveGeneration *struct {
				GenerationID string `json:"generation_id"`
			} `json:"active_generation"`
			LatestGeneration *struct {
				GenerationID string `json:"generation_id"`
			} `json:"latest_generation"`
		}
		_ = doJSON(t, client, ctx, base, http.MethodGet, opPath, token, nil, &opDetail)
		if opDetail.ActiveGeneration != nil {
			lastGenID = opDetail.ActiveGeneration.GenerationID
		} else if opDetail.LatestGeneration != nil {
			lastGenID = opDetail.LatestGeneration.GenerationID
		}
		if lastGenID == "" {
			if time.Now().After(deadline) {
				t.Fatal("no generation id found")
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		code := doJSON(
			t,
			client,
			ctx,
			base,
			http.MethodGet,
			opPath+"/planning-sessions/"+lastGenID,
			token,
			nil,
			&genDetail,
		)
		if code != http.StatusOK {
			t.Fatalf("generation detail: %d", code)
		}
		if genDetail.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("generation not completed in time: %+v", genDetail)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if genDetail.RevisionID == "" {
		t.Fatalf("completed generation missing revision_id: %+v", genDetail)
	}

	// Review the nested immutable revision.
	var rev struct {
		PlanID  string `json:"plan_id"`
		Members []struct {
			MemberID      string `json:"member_id"`
			Excluded      bool   `json:"excluded"`
			Input         string `json:"input"`
			HasComponents bool   `json:"has_components"`
			Task          struct {
				Status    string          `json:"status"`
				Matched   map[string]bool `json:"matched"`
				Unmatched map[string]bool `json:"unmatched"`
			} `json:"task"`
		} `json:"members"`
		Summary struct {
			SummaryReason string `json:"summary_reason"`
		} `json:"summary"`
		Task struct {
			Kind    string          `json:"kind"`
			Payload json.RawMessage `json:"payload"`
		} `json:"task"`
	}
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		opPath+"/revisions/"+genDetail.RevisionID,
		token,
		nil,
		&rev,
	); code != http.StatusOK {
		t.Fatalf("revision detail: %d", code)
	}
	if rev.Summary.SummaryReason == "" {
		t.Fatalf("revision missing summary_reason: %+v", rev)
	}
	if rev.Task.Kind != "conversion" || len(rev.Task.Payload) == 0 {
		t.Fatalf("revision missing task payload: %+v", rev)
	}
	if len(rev.Members) != 1 {
		t.Fatalf("revision members = %+v", rev.Members)
	}
	conclusion := rev.Members[0]
	if conclusion.MemberID == "" || conclusion.Excluded || conclusion.Input != "valid" ||
		!conclusion.HasComponents ||
		conclusion.Task.Status == "" {
		t.Fatalf("revision member conclusion = %+v", conclusion)
	}
	if len(conclusion.Task.Matched) != 4 || len(conclusion.Task.Unmatched) != 4 {
		t.Fatalf("missing partition facts: %+v", conclusion.Task)
	}

	// Member review resolves ownership and returns only albumA's complete inputs.
	var review struct {
		Roots []struct {
			RootPath string `json:"root_path"`
		} `json:"roots"`
		Task struct {
			Payload struct {
				Components []struct {
					ComponentID string `json:"component_id"`
				} `json:"components"`
			} `json:"payload"`
		} `json:"task"`
	}
	if code := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		opPath+"/revisions/"+genDetail.RevisionID+"/members/"+conclusion.MemberID,
		token,
		nil,
		&review,
	); code != http.StatusOK {
		t.Fatalf("member review: %d", code)
	}
	if len(review.Roots) != 1 || !strings.HasSuffix(review.Roots[0].RootPath, "/RJ000001 albumA") ||
		len(review.Task.Payload.Components) == 0 {
		t.Fatalf("member ownership: %+v", review)
	}
	for _, component := range review.Task.Payload.Components {
		if component.ComponentID == "" {
			t.Fatal("member component has no identity")
		}
	}

	// Unchanged generation returns created:false with the same revision.
	var replay struct {
		Created  bool `json:"created"`
		Revision struct {
			PlanID string `json:"plan_id"`
		} `json:"revision"`
	}
	code := doJSONWithHeaders(
		t,
		client,
		ctx,
		base,
		http.MethodPost,
		opPath+"/revisions",
		token,
		map[string]string{"Idempotency-Key": "gen-key-2", "If-Match": "1"},
		nil,
		&replay,
	)
	if code != http.StatusOK || replay.Created || replay.Revision.PlanID != genDetail.RevisionID {
		t.Fatalf("replay: code=%d replay=%+v", code, replay)
	}

	// Delete the library (no active generation) succeeds and orphans the workset.
	if delCode := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodDelete,
		"/api/v1/libraries/"+lib.ID,
		token,
		nil,
		nil,
	); delCode != http.StatusNoContent {
		t.Fatalf("delete library: %d", delCode)
	}
	// The library is gone with its record: the current-record read answers
	// "none", the old record is not addressable, and its plans went with it
	// (ADR 0001). The media on disk is untouched.
	var orphan struct {
		Operations []struct {
			OperationType string `json:"operation_type"`
		} `json:"operations"`
	}
	if getCode := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		wsPath,
		token,
		nil,
		&orphan,
	); getCode != http.StatusNotFound {
		t.Fatalf("a deleted library's record must not resolve: %d", getCode)
	}
	var current struct {
		Workset *json.RawMessage `json:"workset"`
	}
	if getCode := doJSON(
		t,
		client,
		ctx,
		base,
		http.MethodGet,
		"/api/v1/libraries/"+lib.ID+"/operations/conversion/current",
		token,
		nil,
		&current,
	); getCode != http.StatusNotFound {
		t.Fatalf("a deleted library must not answer for a record: %d", getCode)
	}

	// Deleting the library moved no media: the album folders are still there.
	for _, name := range []string{"test1.mp3", "test1.flac"} {
		if _, statErr := os.Stat(filepath.Join(rootPath, "RJ000001 albumA", name)); statErr != nil {
			t.Fatalf("library deletion touched media: %v", statErr)
		}
	}

	// The removed record rejects writes by not existing.
	var draftResp struct {
		Code string `json:"code"`
	}
	code = doJSONWithHeaders(t, client, ctx, base, http.MethodPut,
		opPath+"/draft", token, map[string]string{"If-Match": "1"},
		map[string]any{
			"schema_version": 1,
			"mode":           "available_sources",
			"no_effect_tags": []string{"SEなし"},
			"matched":        map[string]any{"lossless": map[string]any{"codec": "wav"}},
			"unmatched":      map[string]any{"lossless": map[string]any{"codec": "wav"}},
			"members":        []any{},
		}, &draftResp)
	if code != http.StatusNotFound {
		t.Fatalf("a deleted record must reject writes: code=%d resp=%+v", code, draftResp)
	}
}

// doJSONWithHeaders is doJSON with extra request headers (If-Match, Idempotency-Key).
func doJSONWithHeaders(
	t *testing.T,
	client *http.Client,
	ctx context.Context,
	base, method, path, token string,
	headers map[string]string,
	body any,
	out any,
) int {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if out != nil {
		raw, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(raw, out); err != nil {
			t.Logf("raw body: %s", raw)
			t.Fatalf("decode: %v", err)
		}
	}
	return resp.StatusCode
}
