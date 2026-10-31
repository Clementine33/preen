package e2e //nolint:testpackage // shares the backend-binary HTTP harness

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// executionAlbum supplies actual source, replacement and obsolete media.
func executionAlbum(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	wav := filepath.Join(dir, "00.wav")
	mustFFmpegEncode(t, "-f", "lavfi", "-i", "sine=frequency=440:duration=0.4",
		"-metadata", "title=プリセット 曲", "-metadata", "artist=Onsei E2E", wav)
	mustFFmpegEncode(t, "-i", wav, "-b:a", "128k", filepath.Join(dir, "00.mp3"))
	mustFFmpegEncode(t, "-i", wav, "-c:a", "aac", "-b:a", "128k", filepath.Join(dir, "00.m4a"))
	return dir
}

func executionDraft(codec string, bitrate int, deleteMode string) map[string]any {
	profile := map[string]any{
		"lossless": map[string]any{"codec": "wav"},
		"encoded": map[string]any{
			"codec":   codec,
			"quality": map[string]any{"kind": "bitrate", "bitrate": bitrate},
		},
	}
	return map[string]any{
		"schema_version": 1, "mode": "available_sources", "delete_mode": deleteMode,
		"no_effect_tags": []string{"SEなし"}, "matched": profile, "unmatched": profile,
	}
}

// start runs the revision, scoped to the named member folders when any are
// given: through member_ids for the listed ones, or all_except_member_ids for
// every other member of the record.
func (f *execFixtureHTTP) start(t *testing.T, folders ...string) executionView {
	t.Helper()
	return f.startScoped(t, "member_ids", folders...)
}

func (f *execFixtureHTTP) startScoped(t *testing.T, field string, folders ...string) executionView {
	t.Helper()
	body := map[string]any{}
	if len(folders) > 0 {
		var ws struct {
			Members []struct {
				MemberID string `json:"member_id"`
				RelPath  string `json:"rel_path"`
			} `json:"members"`
		}
		if code := doJSON(
			t,
			f.client,
			f.ctx,
			f.base,
			http.MethodGet,
			f.wsPath,
			f.token,
			nil,
			&ws,
		); code != http.StatusOK {
			t.Fatalf("read record: %d", code)
		}
		listed := map[string]bool{}
		for _, name := range folders {
			listed[name] = true
		}
		ids := []string{}
		for _, m := range ws.Members {
			if listed[m.RelPath] {
				ids = append(ids, m.MemberID)
			}
		}
		body[field] = ids
	}
	var started struct {
		Execution executionView `json:"execution"`
	}
	if code := doJSONWithHeaders(t, f.client, f.ctx, f.base, http.MethodPost,
		f.opPath+"/revisions/"+f.revision+"/executions", f.token,
		map[string]string{"Idempotency-Key": "exec-mode", "If-Match": strconv.Itoa(f.opVersion)},
		body, &started); code != http.StatusAccepted {
		t.Fatalf("start execution: %d", code)
	}
	return started.Execution
}

func requireFile(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requireMissing(t *testing.T, file string) {
	t.Helper()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("%s should be absent: %v", file, err)
	}
}

func TestHTTPWorksetExecutionHardDelete(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "music")
	const name = "RJ000001 albumA"
	album := executionAlbum(t, root, name)
	wav := requireFile(t, filepath.Join(album, "00.wav"))
	mp3 := requireFile(t, filepath.Join(album, "00.mp3"))
	f := setupScannedWorkset(t, buildBackendBinary(t), dataDir, root,
		[]string{name}, executionDraft("mp3", 320, "hard"))
	started := f.start(t)
	done := waitForTerminalExecution(t, f.client, f.ctx, f.base, f.opPath, started.ExecutionID, f.token)
	if done.Status != "succeeded" || done.Options.DeleteMode != "hard" {
		t.Fatalf("hard-delete session = %+v", done)
	}
	if !bytes.Equal(wav, requireFile(t, filepath.Join(album, "00.wav"))) ||
		bytes.Equal(mp3, requireFile(t, filepath.Join(album, "00.mp3"))) {
		t.Fatal("the source must stay intact and the mp3 must be replaced")
	}
	requireMissing(t, filepath.Join(album, "00.m4a"))
	if err := filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() && entry.Name() == "Delete" {
			t.Errorf("hard deletion created recovery directory %s", file)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, component := range done.Components {
		if len(component.Recovery) != 0 {
			t.Fatalf("hard deletion reported recovery files: %v", component.Recovery)
		}
	}
}

func TestHTTPWorksetExecutionScopedFolders(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "music")
	const leftName, chosenName = "RJ000001 albumA", "RJ000002 albumB"
	left := executionAlbum(t, root, leftName)
	chosen := executionAlbum(t, root, chosenName)
	before := map[string][]byte{}
	for _, file := range []string{"00.wav", "00.mp3", "00.m4a"} {
		before[file] = requireFile(t, filepath.Join(left, file))
	}
	f := setupScannedWorkset(t, buildBackendBinary(t), dataDir, root, []string{leftName, chosenName}, nil)
	started := f.startScoped(t, "all_except_member_ids", leftName)
	done := waitForTerminalExecution(t, f.client, f.ctx, f.base, f.opPath, started.ExecutionID, f.token)
	if done.Status != "succeeded" || len(done.Components) != 1 ||
		done.Components[0].RootPath != filepath.ToSlash(chosen) {
		t.Fatalf("scoped execution = %+v", done)
	}
	var scope struct {
		SelectedMemberIDs []string `json:"selected_member_ids"`
	}
	if code := doJSON(t, f.client, f.ctx, f.base, http.MethodGet,
		f.opPath+"/executions/"+started.ExecutionID, f.token, nil, &scope); code != http.StatusOK ||
		len(scope.SelectedMemberIDs) != 1 {
		t.Fatalf("selected member ids = %d %+v", code, scope)
	}
	for file, data := range before {
		if !bytes.Equal(data, requireFile(t, filepath.Join(left, file))) {
			t.Fatalf("unselected file changed: %s", file)
		}
	}
	requireMissing(t, filepath.Join(root, "Delete", leftName))
	requireMissing(t, filepath.Join(chosen, "00.m4a"))
	_ = requireFile(t, filepath.Join(root, "Delete", chosenName, "00.m4a"))
	var conflict struct {
		Code    string   `json:"code"`
		Details []string `json:"details"`
	}
	if code := doJSONWithHeaders(t, f.client, f.ctx, f.base, http.MethodPost,
		f.opPath+"/revisions/"+f.revision+"/executions", f.token,
		map[string]string{"Idempotency-Key": "exec-other", "If-Match": strconv.Itoa(f.opVersion)},
		map[string]any{}, &conflict); code != http.StatusConflict ||
		conflict.Code != "PLAN_NOT_EXECUTABLE" {
		t.Fatalf("second execution = %d %+v", code, conflict)
	}
	var spent bool
	for _, detail := range conflict.Details {
		spent = spent || detail == "ALREADY_EXECUTED"
	}
	if !spent {
		t.Fatalf("second execution details = %v", conflict.Details)
	}
}

func TestHTTPWorksetExecutionOpusPreset(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "music")
	const name = "RJ000001 albumA"
	album := executionAlbum(t, root, name)
	f := setupScannedWorkset(t, buildBackendBinary(t), dataDir, root,
		[]string{name}, executionDraft("opus", 160, "soft"))
	started := f.start(t)
	done := waitForTerminalExecution(t, f.client, f.ctx, f.base, f.opPath, started.ExecutionID, f.token)
	if done.Status != "succeeded" {
		t.Fatalf("opus session = %+v", done)
	}
	opus := filepath.Join(album, "00.opus")
	_ = requireFile(t, opus)
	requireMissing(t, filepath.Join(album, "00.mp3"))
	_ = requireFile(t, filepath.Join(root, "Delete", name, "00.mp3"))
	// The setting selects Opus VBR; an average bitrate is not its target setting.
	probe := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_entries",
		"stream=codec_name:stream_tags:format_tags", "-of", "json", opus)
	raw, err := probe.Output()
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Streams []struct {
			Codec string            `json:"codec_name"`
			Tags  map[string]string `json:"tags"`
		} `json:"streams"`
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Streams) != 1 || output.Streams[0].Codec != "opus" {
		t.Fatalf("output codec = %s", raw)
	}
	tags := output.Streams[0].Tags
	if tags["title"] == "" {
		tags = output.Format.Tags
	}
	if tags["title"] != "プリセット 曲" || tags["artist"] != "Onsei E2E" {
		t.Fatalf("output tags = %v", tags)
	}
}

func TestHTTPWorksetExecutionCancelEndpoint(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "music")
	names := []string{"RJ000001 albumA", "RJ000002 albumB"}
	before := map[string][]byte{}
	for _, name := range names {
		album := executionAlbum(t, root, name)
		before[filepath.ToSlash(album)] = requireFile(t, filepath.Join(album, "00.mp3"))
	}
	f := setupScannedWorkset(t, buildBackendBinary(t), dataDir, root, names, nil)
	started := f.start(t)
	path := f.opPath + "/executions/" + started.ExecutionID + "/cancel"
	var canceled executionView
	if code := doJSON(
		t,
		f.client,
		f.ctx,
		f.base,
		http.MethodPost,
		path,
		f.token,
		nil,
		&canceled,
	); code != http.StatusOK ||
		canceled.ExecutionID != started.ExecutionID {
		t.Fatalf("cancel = %d %+v", code, canceled)
	}
	done := waitForTerminalExecution(t, f.client, f.ctx, f.base, f.opPath, started.ExecutionID, f.token)
	// Finishing before cancel arrives is valid. Safe-boundary cancellation is
	// controlled deterministically in the execution tests, not by slower media.
	if done.Status != "canceled" && done.Status != "succeeded" {
		t.Fatalf("cancel terminal = %+v", done)
	}
	if len(done.Components) != len(names) {
		t.Fatalf("cancel lost the frozen worklist: %+v", done)
	}
	for _, component := range done.Components {
		for _, file := range component.Committed {
			_ = requireFile(t, filepath.FromSlash(file))
		}
		if component.Status == "pending" {
			file := filepath.Join(filepath.FromSlash(component.RootPath), "00.mp3")
			if !bytes.Equal(before[component.RootPath], requireFile(t, file)) {
				t.Fatalf("pending component changed %s", file)
			}
		}
	}
	if code := doJSON(
		t,
		f.client,
		f.ctx,
		f.base,
		http.MethodPost,
		path,
		f.token,
		nil,
		&canceled,
	); code != http.StatusOK || canceled.Status != done.Status ||
		canceled.ExecutionID != done.ExecutionID {
		t.Fatalf("terminal cancel replay = %d %+v", code, canceled)
	}
}
