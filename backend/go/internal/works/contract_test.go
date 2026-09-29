package works_test

import (
	"encoding/json"
	"testing"

	"github.com/onsei/organizer/backend/internal/works"
)

//go:fix inline

//go:fix inline

// TestCursorRoundTrip covers every shape the listing writes: a known value, and
// the unknown block of a nullable order, which carries no value at all.
func TestCursorRoundTrip(t *testing.T) {
	for _, want := range []works.Cursor{
		{Sort: works.SortByReleaseDate, Direction: works.Descending, SortValue: "2025-06-25"},
		{Sort: works.SortByReleaseDate, Direction: works.Ascending, NullRank: 1},
		{Sort: works.SortByRating, Direction: works.Descending, SortValue: "8"},
		{Sort: works.SortByRating, Direction: works.Descending, NullRank: 1},
	} {
		want.Digest = "0123456789abcdef"
		want.WorkID = code("RJ01657200")
		got, err := works.DecodeCursor(want.Encode())
		if err != nil {
			t.Fatalf("DecodeCursor refused a cursor this package wrote (%+v): %v", want, err)
		}
		if got != want {
			t.Errorf("round trip changed the cursor: %+v -> %+v", want, got)
		}
	}
}

func TestDecodeCursorRefusesForeignInput(t *testing.T) {
	valid := works.Cursor{
		Sort:      works.SortByWorkID,
		Direction: works.Ascending,
		Digest:    "0123456789abcdef",
		SortValue: "01657200",
		WorkID:    code("RJ01657200"),
	}.Encode()

	for _, tc := range []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"not enough fields", "v1|rj|asc|0123456789abcdef|0|01657200"},
		{"too many fields", valid + "|extra"},
		{"unknown version", "v2|rj|asc|0123456789abcdef|0|01657200|RJ01657200"},
		{"unknown sort", "v1|title|asc|0123456789abcdef|0|01657200|RJ01657200"},
		{"unknown direction", "v1|rj|sideways|0123456789abcdef|0|01657200|RJ01657200"},
		{"bad null rank", "v1|rj|asc|0123456789abcdef|2|01657200|RJ01657200"},
		{"empty sort value", "v1|rj|asc|0123456789abcdef|0||RJ01657200"},
		{"unknown block of a total order", "v1|rj|asc|0123456789abcdef|1||RJ01657200"},
		{"value inside the unknown block", "v1|release_date|asc|0123456789abcdef|1|2025-06-25|RJ01657200"},
		{"rating out of range", "v1|rating|desc|0123456789abcdef|0|11|RJ01657200"},
		{"rating not a number", "v1|rating|desc|0123456789abcdef|0|high|RJ01657200"},
		{"not a work number", "v1|rj|asc|0123456789abcdef|0|01657200|nonsense"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := works.DecodeCursor(tc.in); err == nil {
				t.Fatalf("DecodeCursor(%q) accepted a cursor it did not write", tc.in)
			}
		})
	}
}

// TestCursorIsBoundToItsQuery: a cursor carries a fingerprint of the query, so
// changing a filter or the ordering invalidates it instead of silently paging
// through a different sequence.
func TestCursorIsBoundToItsQuery(t *testing.T) {
	base := works.Filter{Sort: works.SortByWorkID, Direction: works.Ascending}
	baseCursor := works.Cursor{
		Sort:      base.Sort,
		Direction: base.Direction,
		Digest:    base.Digest(),
		SortValue: "01657200",
		WorkID:    code("RJ01657200"),
	}
	if err := baseCursor.Matches(base); err != nil {
		t.Fatalf("the cursor did not match its own query: %v", err)
	}

	changedFilter := base
	changedFilter.Query = new("asmr")
	if err := baseCursor.Matches(changedFilter); err == nil {
		t.Error("a changed filter accepted the old cursor")
	}

	changedSort := base
	changedSort.Direction = works.Descending
	if err := baseCursor.Matches(changedSort); err == nil {
		t.Error("a changed direction accepted the old cursor")
	}

	// The page size is not part of the fingerprint: continuing a scan with a
	// different page size is legitimate.
	changedLimit := base
	changedLimit.Limit = 10
	if err := baseCursor.Matches(changedLimit); err != nil {
		t.Errorf("a changed page size should not invalidate the cursor: %v", err)
	}
}

// TestFilterDigestIgnoresSetOrder: the same set written in another order is the
// same query.
func TestFilterDigestIgnoresSetOrder(t *testing.T) {
	first := works.Filter{Circles: []string{"c1", "c2"}, Languages: []string{"JPN", "CHI_HANS"}}
	second := works.Filter{Circles: []string{"c2", "c1"}, Languages: []string{"CHI_HANS", "JPN"}}
	if first.Digest() != second.Digest() {
		t.Error("the same filter written in another order produced a different digest")
	}
}

func TestFilterValidation(t *testing.T) {
	cases := []struct {
		name string
		in   works.Filter
		code string
	}{
		{"limit below one", works.Filter{Limit: -1}, works.CodeInvalidPage},
		{"limit above the maximum", works.Filter{Limit: 201}, works.CodeInvalidPage},
		{"unknown sort", works.Filter{Sort: "title"}, works.CodeInvalidArgument},
		{"unknown direction", works.Filter{Direction: "sideways"}, works.CodeInvalidArgument},
		{"unknown nsfw", works.Filter{NSFW: "maybe"}, works.CodeInvalidArgument},
		{"set without match mode", works.Filter{VoiceActors: []string{"v1"}}, works.CodeMatchModeRequired},
		{"match mode without a set", works.Filter{VoiceActorMatch: works.MatchAll}, works.CodeInvalidArgument},
		{"rating above the range", works.Filter{RatingFrom: new(11)}, works.CodeInvalidArgument},
		{"inverted rating range", works.Filter{RatingFrom: new(8), RatingTo: new(3)}, works.CodeInvalidArgument},
		{"malformed date", works.Filter{ReleasedFrom: new("2025/06/25")}, works.CodeInvalidArgument},
		{
			"inverted date range",
			works.Filter{ReleasedFrom: new("2026-01-01"), ReleasedTo: new("2025-01-01")},
			works.CodeInvalidArgument,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := tc.in
			err := filter.Validate()
			if err == nil {
				t.Fatalf("%+v was accepted", tc.in)
			}
			if err.Code != tc.code {
				t.Errorf("code = %q, want %q", err.Code, tc.code)
			}
		})
	}
}

func TestFilterDefaults(t *testing.T) {
	filter := works.Filter{}
	if err := filter.Validate(); err != nil {
		t.Fatalf("an empty filter was refused: %v", err)
	}
	if filter.Limit != works.DefaultLimit {
		t.Errorf("limit = %d, want %d", filter.Limit, works.DefaultLimit)
	}
	if filter.Sort != works.SortByWorkID || filter.Direction != works.Ascending {
		t.Errorf("defaults = %s/%s, want rj/asc", filter.Sort, filter.Direction)
	}
}

// TestDecideGroup pins the four-group source rules: a higher-priority source
// replaces a lower one, the same source only moves forward in time, and sales
// have no source to arbitrate.
func TestDecideGroup(t *testing.T) {
	stored := works.SourceState{Source: works.SourceDLsite, FetchedAt: 200}

	cases := []struct {
		name     string
		group    works.DataGroup
		incoming works.Source
		fetched  int64
		existing works.SourceState
		present  bool
		apply    bool
		reason   string
	}{
		{"nothing stored", works.GroupCore, works.SourceASMRone, 100, works.SourceState{}, false, true, ""},
		{"newer from the same source", works.GroupCore, works.SourceDLsite, 300, stored, true, true, ""},
		{"same fetch time", works.GroupCore, works.SourceDLsite, 200, stored, true, false, works.ReasonStaleFetchedAt},
		{
			"older from the same source",
			works.GroupCore,
			works.SourceDLsite,
			100,
			stored,
			true,
			false,
			works.ReasonStaleFetchedAt,
		},
		{
			"lower priority never overwrites", works.GroupCore, works.SourceASMRone, 300, stored, true,
			false, works.ReasonLowerPrioritySource,
		},
		{
			"higher priority replaces", works.GroupCore, works.SourceDLsite, 300,
			works.SourceState{Source: works.SourceASMRone, FetchedAt: 400}, true, true, "",
		},
		{
			"sales only compare fetch time", works.GroupStatistics, works.SourceDLsite, 100, stored, true,
			false, works.ReasonStaleFetchedAt,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := works.DecideGroup(tc.group, tc.incoming, tc.fetched, tc.existing, tc.present)
			if got.Apply != tc.apply {
				t.Fatalf("Apply = %v, want %v (%s)", got.Apply, tc.apply, got.Message)
			}
			if got.Reason != tc.reason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.reason)
			}
		})
	}
}

// TestUserDataRatingIsAnIntegerOneToTen covers the two halves of the rule: the
// range is checked with a clear refusal, and an explicit null clears the value
// while an absent field leaves it alone.
func TestUserDataRatingIsAnIntegerOneToTen(t *testing.T) {
	for _, value := range []int{0, 11, -1} {
		req := works.UserDataRequest{Rating: works.NullableInt{Set: true, Value: new(value)}}
		if _, err := req.Patch(); err == nil {
			t.Errorf("rating %d was accepted", value)
		}
	}
	if _, err := (works.UserDataRequest{Rating: works.NullableInt{Set: true, Value: new(7)}}).Patch(); err != nil {
		t.Errorf("rating 7 was refused: %v", err)
	}

	cleared, err := (works.UserDataRequest{Rating: works.NullableInt{Set: true, Value: nil}}).Patch()
	if err != nil {
		t.Fatalf("an explicit null rating was refused: %v", err)
	}
	if !cleared.Rating.Set || cleared.Rating.Value != nil {
		t.Errorf("explicit null produced %+v, want a set-and-empty rating", cleared.Rating)
	}

	untouched, err := (works.UserDataRequest{}).Patch()
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Rating.Set || untouched.Favorite.Set || untouched.Note.Set {
		t.Errorf("an empty patch set fields: %+v", untouched)
	}
}

// TestUserDataRatingRejectsAFraction: a JSON number with a fraction is not an
// integer rating, and the decoder must not round it into one.
func TestUserDataRatingRejectsAFraction(t *testing.T) {
	var req works.UserDataRequest
	if err := json.Unmarshal([]byte(`{"rating":7.5}`), &req); err == nil {
		t.Errorf("rating 7.5 was accepted as %+v", req.Rating)
	}
}
