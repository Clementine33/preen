package works

import "strconv"

// Page size bounds for the ordinary listing.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// MatchMode is how a set of ids is matched: every one, or at least one.
type MatchMode string

// The two set match modes.
const (
	MatchAll MatchMode = "all"
	MatchAny MatchMode = "any"
)

// Valid reports whether the mode is one this build accepts.
func (m MatchMode) Valid() bool { return m == MatchAll || m == MatchAny }

// NSFWFilter is the three-way adult-content filter: unknown is a value, not a
// synonym for false.
type NSFWFilter string

// The accepted adult-content filters.
const (
	NSFWAny      NSFWFilter = ""
	NSFWOnly     NSFWFilter = "true"
	NSFWExcluded NSFWFilter = "false"
	NSFWUnknown  NSFWFilter = "unknown"
)

// Valid reports whether the filter is one this build accepts.
func (n NSFWFilter) Valid() bool {
	switch n {
	case NSFWAny, NSFWOnly, NSFWExcluded, NSFWUnknown:
		return true
	default:
		return false
	}
}

// SortField is the column the listing is ordered by.
type SortField string

// The sortable fields. Nothing else is sortable, so the order-by expression is
// chosen from a whitelist rather than built from the request.
const (
	SortByWorkID      SortField = "rj"
	SortByReleaseDate SortField = "release_date"
	SortByCollectedAt SortField = "collected_at"
	// SortByRating orders by the user's rating; unrated works always come last,
	// the way unknown release dates do.
	SortByRating SortField = "rating"
)

// Valid reports whether the field is sortable.
func (s SortField) Valid() bool {
	switch s {
	case SortByWorkID, SortByReleaseDate, SortByCollectedAt, SortByRating:
		return true
	default:
		return false
	}
}

// Direction is the sort direction.
type Direction string

// The two directions.
const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

// Valid reports whether the direction is one this build accepts.
func (d Direction) Valid() bool { return d == Ascending || d == Descending }

// Filter is the ordinary listing's whole query surface. Every dimension is
// optional; the dimensions intersect with each other, while the rating range
// and the not-rated switch union inside their own dimension.
type Filter struct {
	Query *string

	Circles         []string
	VoiceActors     []string
	VoiceActorMatch MatchMode
	SourceTags      []string
	SourceTagMatch  MatchMode
	UserTags        []string
	UserTagMatch    MatchMode
	// TagGroups combine both vocabularies, one group per picked tag (see
	// TagGroup); TagMatch combines the groups.
	TagGroups []TagGroup
	TagMatch  MatchMode

	Languages   []string
	NSFW        NSFWFilter
	AgeCategory *int

	ReleasedFrom *string
	ReleasedTo   *string

	RatingFrom *int
	RatingTo   *int
	Unrated    *bool

	Favorite  *bool
	Libraries []string
	HasCopy   *bool

	Sort      SortField
	Direction Direction
	Limit     int

	// Page is the 1-based page of Limit works to answer; 0 means the first.
	Page int
}

// Validate applies the filter contract: every set filter says how to match, a
// range is ordered and within bounds, and the page size is inside its own
// limits. Everything it refuses is a 400, never a silent default.
func (f *Filter) Validate() *Error {
	if err := f.validateWindow(); err != nil {
		return err
	}
	if f.Sort == "" {
		f.Sort = SortByWorkID
	}
	if !f.Sort.Valid() {
		return Invalid(CodeInvalidArgument, "unknown sort field")
	}
	if f.Direction == "" {
		f.Direction = Ascending
	}
	if !f.Direction.Valid() {
		return Invalid(CodeInvalidArgument, "direction must be asc or desc")
	}
	if !validAgeCategory(f.AgeCategory) {
		return Invalid(CodeInvalidArgument, "age_category must be 1, 2 or 3")
	}
	if !f.NSFW.Valid() {
		return Invalid(CodeInvalidArgument, "nsfw must be true, false or unknown")
	}

	// A set filter without a match mode has no defensible default: choosing
	// one would silently answer a different question than the caller asked.
	for _, set := range []struct {
		name    string
		present bool
		match   MatchMode
	}{
		{"voice_actor", len(f.VoiceActors) > 0, f.VoiceActorMatch},
		{"source_tag", len(f.SourceTags) > 0, f.SourceTagMatch},
		{"user_tag", len(f.UserTags) > 0, f.UserTagMatch},
		{"tag", len(f.TagGroups) > 0, f.TagMatch},
	} {
		if !set.present {
			if set.match != "" {
				return Invalid(CodeInvalidArgument, set.name+"_match needs "+set.name)
			}
			continue
		}
		if set.match == "" {
			return &Error{
				Kind:    KindInvalid,
				Code:    CodeMatchModeRequired,
				Message: set.name + "_match must be all or any",
			}
		}
		if !set.match.Valid() {
			return Invalid(CodeInvalidArgument, set.name+"_match must be all or any")
		}
	}

	if f.RatingFrom != nil && (*f.RatingFrom < 1 || *f.RatingFrom > 10) {
		return Invalid(CodeInvalidArgument, "rating_from must be between 1 and 10")
	}
	if f.RatingTo != nil && (*f.RatingTo < 1 || *f.RatingTo > 10) {
		return Invalid(CodeInvalidArgument, "rating_to must be between 1 and 10")
	}
	if f.RatingFrom != nil && f.RatingTo != nil && *f.RatingFrom > *f.RatingTo {
		return Invalid(CodeInvalidArgument, "rating_from must not exceed rating_to")
	}
	for _, date := range []struct {
		name  string
		value *string
	}{{"released_from", f.ReleasedFrom}, {"released_to", f.ReleasedTo}} {
		if date.value != nil && !validDate(*date.value) {
			return Invalid(CodeInvalidArgument, date.name+" must be YYYY-MM-DD")
		}
	}
	if f.ReleasedFrom != nil && f.ReleasedTo != nil && *f.ReleasedFrom > *f.ReleasedTo {
		return Invalid(CodeInvalidArgument, "released_from must not be after released_to")
	}
	return nil
}

// validDate reports whether s is a calendar date in the stored fixed width
// form. The comparison is textual, so the width has to be exactly right; it is
// taken from the layout rather than written down twice.
func validDate(s string) bool {
	if len(s) != len(DateFormat) {
		return false
	}
	for i := range len(s) {
		if i == 4 || i == 7 {
			if s[i] != '-' {
				return false
			}
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// validateWindow fills and checks the page window: the page size within its
// bounds and a page number of at least 1, the unset ones taking the defaults.
func (f *Filter) validateWindow() *Error {
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	if f.Page == 0 {
		f.Page = 1
	}
	if f.Page < 1 {
		return Invalid(CodeInvalidPage, "page must be 1 or more")
	}
	if f.Limit < 1 || f.Limit > MaxLimit {
		return Invalid(CodeInvalidPage, "limit must be between 1 and "+strconv.Itoa(MaxLimit))
	}
	return nil
}
