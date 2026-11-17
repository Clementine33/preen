package sqlite

import (
	"strings"
	"testing"
)

func TestMediaFactsQueriesWalkDirectoryAndFileKeys(t *testing.T) {
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	for _, query := range []string{mediaFactsCandidatesSQL, mediaFactsCountsSQL} {
		plan := explain(t, repo, query)
		t.Log(plan)
		if strings.Contains(plan, "SCAN f") || strings.Contains(plan, "TEMP B-TREE FOR ORDER BY") ||
			!strings.Contains(plan, "SEARCH f USING PRIMARY KEY") {
			t.Fatalf("media facts query plan: %s", plan)
		}
	}
}
