package inventory_test

import (
	"slices"
	"testing"

	"github.com/onsei/organizer/backend/internal/inventory"
)

func TestSelectedScopesCollapseAncestorsAcrossSortedNeighbors(t *testing.T) {
	scopes, err := inventory.SelectedScopes(
		"/library",
		[]string{"/library/a/child", "/library/a-sibling", "/library/a", "/library/a/child"},
	)
	if err != nil || !slices.Equal(scopes, []string{"/library/a", "/library/a-sibling"}) {
		t.Fatalf("scopes = %v, %v", scopes, err)
	}
	for _, scope := range []string{"/library", "/library2/a", "", "/library/../outside"} {
		if _, err := inventory.SelectedScopes("/library", []string{scope}); err == nil {
			t.Fatalf("unbounded scope accepted: %s", scope)
		}
	}
}
