package sqlite //nolint:testpackage // reads the unexported batched query to check its plan

import (
	"strings"
	"testing"
)

// TestObservedAudioByRootSearchesThePathIndexPerRoot pins that the batched read
// stays a range search per frozen root instead of scanning the library.
func TestObservedAudioByRootSearchesThePathIndexPerRoot(t *testing.T) {
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	plan := explain(t, repo, observedAudioByRootSQL)
	t.Log(plan)
	if strings.Contains(plan, "SCAN d") || strings.Contains(plan, "SCAN f") ||
		!strings.Contains(plan, "SEARCH d USING") {
		t.Fatalf("batched observed-audio plan: %s", plan)
	}
}
