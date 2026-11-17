package sqlite

import (
	"strings"
	"testing"
)

func TestGenerationRecordRetentionChecksAnInventorySet(t *testing.T) {
	repo := newTestRepository(t)
	plan := explain(t, repo, "SELECT g.path FROM generation_records g WHERE "+generationRecordRetentionPredicate)
	if strings.Contains(plan, "CORRELATED") || strings.Contains(plan, "SCAN f") ||
		!strings.Contains(plan, "SEARCH f USING PRIMARY KEY") {
		t.Fatalf("retention must build inventory paths once: %s", plan)
	}
}
