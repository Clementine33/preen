// packScopes is a pure batching rule with no observable seam short of a merge
// whose scope costs exceed the batch budget.

package sqlite

import (
	"reflect"
	"testing"
)

func TestPackScopesNeverSplitsAndIsolatesOverBudgetScopes(t *testing.T) {
	scopes := []string{"a", "b", "c", "d", "e"}
	got := packScopes(scopes, []int{3, 3, 50, 1, 5}, 7)
	want := [][]string{{"a", "b"}, {"c"}, {"d", "e"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batches = %v, want %v", got, want)
	}
}
