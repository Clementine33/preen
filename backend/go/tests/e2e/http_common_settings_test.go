package e2e //nolint:testpackage // white-box tests exercise unexported internals

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestHTTPCommonSettingsSyncAcrossRecords boots the real backend and covers the
// deployment-wide common-settings rule over HTTP: a draft save that changes the
// common settings reaches another library's record with its member overrides
// and exclusions intact, and a record created afterwards starts from the same
// settings (ADR 0010).
func TestHTTPCommonSettingsSyncAcrossRecords(t *testing.T) {
	binPath := buildBackendBinary(t)
	dataDir := t.TempDir()
	for _, name := range []string{"musicA", "musicB", "musicC"} {
		root := filepath.Join(dataDir, name)
		mustWriteFile(t, filepath.Join(root, "RJ000001 albumA", "test1.mp3"), "dummy audio")
		mustWriteFile(t, filepath.Join(root, "RJ000001 albumA", "test1.flac"), "dummy audio")
		mustWriteFile(t, filepath.Join(root, "RJ000001 albumA", "test2.mp3"), "dummy audio")
	}

	const token = "e2e-token"
	proc := startBackendBinary(t, binPath, dataDir, token)
	base := fmt.Sprintf("http://127.0.0.1:%d", proc.httpPort)
	client := &http.Client{Timeout: 60 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	first := createAndScanLibrary(t, client, ctx, base, token, dataDir, "musicA", "一库")
	second := createAndScanLibrary(t, client, ctx, base, token, dataDir, "musicB", "二库")

	// The second record carries a member override and an excluded member, made
	// through a save that leaves its own common settings exactly as seeded, so
	// nothing global happens yet.
	body := draftPayload(t, client, ctx, base, token, second.worksetID)
	body["members"] = []map[string]any{{
		"member_id": second.memberID,
		"excluded":  true,
		"overrides": map[string]any{"mode": "strict"},
	}}
	saved := putDraft(t, client, ctx, base, token, second.worksetID, 1, body)
	if saved != 2 {
		t.Fatalf("member-only save version = %d, want 2", saved)
	}

	// Changing the common settings from the first record is a deployment-wide
	// save: the second record's common fields move with it.
	firstBody := draftPayload(t, client, ctx, base, token, first.worksetID)
	firstBody["mode"] = "strict"
	firstBody["classifier_tags"] = []string{"反転", "SEあり"}
	firstBody["matched"] = map[string]any{"lossless": map[string]any{"codec": "flac"}}
	if got := putDraft(t, client, ctx, base, token, first.worksetID, 1, firstBody); got != 2 {
		t.Fatalf("global save version = %d, want 2", got)
	}

	synced := draftPayload(t, client, ctx, base, token, second.worksetID)
	if synced["mode"] != "strict" {
		t.Fatalf("synced mode = %v", synced["mode"])
	}
	tags, _ := synced["classifier_tags"].([]any)
	if len(tags) != 2 || tags[0] != "反転" {
		t.Fatalf("synced classifier tags = %v", synced["classifier_tags"])
	}
	matched, _ := synced["matched"].(map[string]any)
	lossless, _ := matched["lossless"].(map[string]any)
	if lossless == nil || lossless["codec"] != "flac" {
		t.Fatalf("synced matched profile = %v", synced["matched"])
	}
	members, _ := synced["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("synced member records = %v, want the stored one", synced["members"])
	}
	member, _ := members[0].(map[string]any)
	if member["excluded"] != true {
		t.Fatalf("the synced record lost the member exclusion: %v", member)
	}
	overrides, _ := member["overrides"].(map[string]any)
	if overrides == nil || overrides["mode"] != "strict" {
		t.Fatalf("the synced record lost the member override: %v", member)
	}
	if version := operationVersion(t, client, ctx, base, token, second.worksetID); version != 3 {
		t.Fatalf("synced operation version = %d, want 3", version)
	}

	// A record created after the save starts from the stored settings.
	third := createAndScanLibrary(t, client, ctx, base, token, dataDir, "musicC", "新库")
	seeded := draftPayload(t, client, ctx, base, token, third.worksetID)
	if seeded["mode"] != "strict" {
		t.Fatalf("a new record seeded mode = %v", seeded["mode"])
	}
	seededTags, _ := seeded["classifier_tags"].([]any)
	if len(seededTags) != 2 || seededTags[0] != "反転" {
		t.Fatalf("a new record seeded classifier tags = %v", seeded["classifier_tags"])
	}
}

// libraryRef is what the test needs from one created library: its record and
// that record's member.
type libraryRef struct {
	worksetID string
	memberID  string
}

// createAndScanLibrary creates one library, scans it, and creates its current
// conversion record over its single album folder.
func createAndScanLibrary(
	t *testing.T,
	client *http.Client,
	ctx context.Context,
	base, token, dataDir, dir, name string,
) libraryRef {
	t.Helper()
	root := filepath.Join(dataDir, dir)
	var lib struct {
		ID string `json:"id"`
	}
	req := map[string]string{"name": name, "root_path": filepath.ToSlash(root)}
	if code := doJSON(
		t, client, ctx, base, http.MethodPost, "/api/v1/libraries", token, req, &lib,
	); code != http.StatusCreated {
		t.Fatalf("create library %s: %d", name, code)
	}
	if events := postScanSSE(t, client, ctx, base, lib.ID, token); events["completed"] == nil {
		t.Fatalf("scan of %s did not complete: %v", name, events)
	}

	var created struct {
		Workset struct {
			WorksetID string `json:"workset_id"`
		} `json:"workset"`
	}
	if code := doJSONWithHeaders(
		t, client, ctx, base, http.MethodPut,
		"/api/v1/libraries/"+lib.ID+"/operations/conversion/current", token,
		map[string]string{"Idempotency-Key": "create-" + dir},
		map[string]any{"folder_paths": []string{"RJ000001 albumA"}}, &created,
	); code != http.StatusCreated {
		t.Fatalf("create record for %s: %d", name, code)
	}
	var record struct {
		Workset struct {
			Members []struct {
				MemberID string `json:"member_id"`
			} `json:"members"`
		} `json:"workset"`
	}
	if code := doJSON(
		t, client, ctx, base, http.MethodGet,
		"/api/v1/libraries/"+lib.ID+"/operations/conversion/current", token, nil, &record,
	); code != http.StatusOK ||
		len(record.Workset.Members) != 1 {
		t.Fatalf("current record of %s: %d %+v", name, code, record)
	}
	return libraryRef{
		worksetID: created.Workset.WorksetID,
		memberID:  record.Workset.Members[0].MemberID,
	}
}

// draftPayload reads one record's draft payload as an editable JSON object: the
// persisted common settings with whatever member records it holds.
func draftPayload(
	t *testing.T,
	client *http.Client,
	ctx context.Context,
	base, token, worksetID string,
) map[string]any {
	t.Helper()
	var draft struct {
		Task struct {
			Payload map[string]any `json:"payload"`
		} `json:"task"`
	}
	if code := doJSON(
		t, client, ctx, base, http.MethodGet,
		"/api/v1/worksets/"+worksetID+"/operations/conversion/draft", token, nil, &draft,
	); code != http.StatusOK {
		t.Fatalf("get draft: %d", code)
	}
	if draft.Task.Payload == nil {
		t.Fatalf("draft payload of %s is empty", worksetID)
	}
	return draft.Task.Payload
}

// putDraft saves one draft document under the given If-Match and returns the
// operation version the save answered with.
func putDraft(
	t *testing.T,
	client *http.Client,
	ctx context.Context,
	base, token, worksetID string,
	ifMatch int,
	body map[string]any,
) int {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal draft: %v", err)
	}
	var saved struct {
		Version int `json:"version"`
	}
	if code := doJSONWithHeaders(
		t, client, ctx, base, http.MethodPut,
		"/api/v1/worksets/"+worksetID+"/operations/conversion/draft", token,
		map[string]string{"If-Match": strconv.Itoa(ifMatch)},
		json.RawMessage(raw), &saved,
	); code != http.StatusOK {
		t.Fatalf("save draft: %d", code)
	}
	return saved.Version
}

// operationVersion reads one operation's current version.
func operationVersion(
	t *testing.T,
	client *http.Client,
	ctx context.Context,
	base, token, worksetID string,
) int {
	t.Helper()
	var op struct {
		Version int `json:"version"`
	}
	if code := doJSON(
		t, client, ctx, base, http.MethodGet,
		"/api/v1/worksets/"+worksetID+"/operations/conversion", token, nil, &op,
	); code != http.StatusOK {
		t.Fatalf("get operation: %d", code)
	}
	return op.Version
}
