package e2e //nolint:testpackage // shares the real backend process and HTTP harness

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestHTTPFirstStartupDeliversDefaultTags(t *testing.T) {
	dir := t.TempDir()
	proc := startBackendBinary(t, buildBackendBinary(t), dir, "", "--watch-stdin")
	base := fmt.Sprintf("http://127.0.0.1:%d", proc.httpPort)
	client := &http.Client{Timeout: 30 * time.Second}
	var tags struct {
		NoEffect []string `json:"default_no_effect_tags"`
		Other    []string `json:"default_other_tags"`
	}
	code := doJSON(t, client, t.Context(), base, http.MethodGet, "/api/v1/classifier-tags", "", nil, &tags)
	if code != http.StatusOK || len(tags.NoEffect) != 13 || !slices.Contains(tags.NoEffect, "nose") ||
		!slices.Equal(tags.Other, []string{"反転", "反转"}) {
		t.Fatalf("initial classifier tags: status=%d, tags=%+v", code, tags)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "internal", "adapters", "settings", "config.json.template"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil || string(got) != string(want) {
		t.Fatalf("initial configuration differs from template: %q, %v", got, err)
	}
	mustWriteFile(t, filepath.Join(dir, "music", "RJ000001 albumA", "test.mp3"), "dummy audio")
	fixture := createAndScanLibrary(t, client, t.Context(), base, "", dir, "music", "initial tags")
	draft := draftPayload(t, client, t.Context(), base, "", fixture.worksetID)
	for key, expected := range map[string][]string{"no_effect_tags": tags.NoEffect, "other_tags": tags.Other} {
		values, ok := draft[key].([]any)
		if !ok || len(values) != len(expected) {
			t.Fatalf("draft %s = %v, want %v", key, draft[key], expected)
		}
		for i, tag := range expected {
			if values[i] != tag {
				t.Fatalf("draft %s = %v, want %v", key, values, expected)
			}
		}
	}
}
