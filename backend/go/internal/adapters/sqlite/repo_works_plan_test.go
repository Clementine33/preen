package sqlite //nolint:testpackage // white-box test reads the plans the listing's SQL produces

import (
	"fmt"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/works"
)

// TestListingOrderingsAreServedByIndexes is the query-plan check the spec asks
// for: every supported ordering must read an index in order instead of sorting
// the whole matching set. A temp B-tree here would mean a page costs a sort per
// request, which is the difference between a seek and an O(n log n) pass over
// the catalog.
//
// The assertion is about the property, not about SQLite's wording: the plan
// must name an index and must not build a temporary B-tree. That holds across
// planner versions in a way a pinned plan string would not.
func TestListingOrderingsAreServedByIndexes(t *testing.T) {
	repo := newTestRepository(t)
	for i := 1; i <= 30; i++ {
		seedWork(t, repo, "RJ000000"+twoDigits(i), true)
	}

	cases := []struct {
		name   string
		filter works.Filter
	}{
		{"by work number, ascending", works.Filter{Sort: works.SortByWorkID, Direction: works.Ascending}},
		{"by work number, descending", works.Filter{Sort: works.SortByWorkID, Direction: works.Descending}},
		{"by release date, ascending", works.Filter{Sort: works.SortByReleaseDate, Direction: works.Ascending}},
		{"by release date, descending", works.Filter{Sort: works.SortByReleaseDate, Direction: works.Descending}},
		{"by collection time, ascending", works.Filter{Sort: works.SortByCollectedAt, Direction: works.Ascending}},
		{"by collection time, descending", works.Filter{Sort: works.SortByCollectedAt, Direction: works.Descending}},
		{"by release date, a deep page", works.Filter{
			Sort: works.SortByReleaseDate, Direction: works.Descending, Page: 3, Limit: 10,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := listingPlan(t, repo, tc.filter)
			joined := strings.Join(plan, "\n")
			if strings.Contains(joined, "TEMP B-TREE") {
				t.Errorf("the ordering is sorted rather than indexed:\n%s", joined)
			}
			if !strings.Contains(joined, "USING") {
				t.Errorf("no index is used at all:\n%s", joined)
			}
		})
	}
}

// listingPlan returns the plan of the ordinary listing as the adapter builds it
// for one filter.
func listingPlan(t *testing.T, repo *Repository, filter works.Filter) []string {
	t.Helper()
	if err := filter.Validate(); err != nil {
		t.Fatalf("filter: %v", err)
	}
	query, args := listingSQL(filter)
	rows, err := repo.DB().Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 {
		t.Fatal("the planner reported no steps")
	}
	return plan
}

// twoDigits renders 1..99 as two digits, so a seeded work number stays valid.
func twoDigits(value int) string {
	return fmt.Sprintf("%02d", value)
}
