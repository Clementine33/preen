package works

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

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

// Nullable reports whether the field can be unknown, which puts a trailing
// block of unknown rows after every known one, in either direction.
func (s SortField) Nullable() bool { return s == SortByReleaseDate || s == SortByRating }

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

	// Cursor continues a previous page. It is not part of Digest - it is the
	// position within the query, not a description of it.
	Cursor *Cursor
}

// Validate applies the filter contract: every set filter says how to match, a
// range is ordered and within bounds, and the page size is inside its own
// limits. Everything it refuses is a 400, never a silent default.
func (f *Filter) Validate() *Error {
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit < 1 || f.Limit > MaxLimit {
		return &Error{
			Kind:    KindInvalid,
			Code:    CodeInvalidPage,
			Message: "limit must be between 1 and " + strconv.Itoa(MaxLimit),
		}
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
		name  string
		ids   []string
		match MatchMode
	}{
		{"voice_actor", f.VoiceActors, f.VoiceActorMatch},
		{"source_tag", f.SourceTags, f.SourceTagMatch},
		{"user_tag", f.UserTags, f.UserTagMatch},
	} {
		if len(set.ids) == 0 {
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

// Digest is the query fingerprint a cursor carries: everything that decides
// which works are in the result and in what order. The page size is
// deliberately not part of it - continuing a scan with a different page size
// is a legitimate thing to do, and turning it into a 400 would only be
// annoying.
func (f *Filter) Digest() string {
	parts := make([]string, 0, 16)
	add := func(name, value string) {
		if value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	addSet := func(name string, values []string) {
		if len(values) == 0 {
			return
		}
		sorted := append([]string(nil), values...)
		sort.Strings(sorted)
		add(name, strings.Join(sorted, ","))
	}
	point := func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}
	intPoint := func(value *int) string {
		if value == nil {
			return ""
		}
		return strconv.Itoa(*value)
	}
	boolPoint := func(value *bool) string {
		if value == nil {
			return ""
		}
		return strconv.FormatBool(*value)
	}
	if f.Query != nil {
		add("q", *f.Query)
	}
	addSet("circle", f.Circles)
	addSet("voice_actor", f.VoiceActors)
	add("voice_actor_match", string(f.VoiceActorMatch))
	addSet("source_tag", f.SourceTags)
	add("source_tag_match", string(f.SourceTagMatch))
	addSet("user_tag", f.UserTags)
	add("user_tag_match", string(f.UserTagMatch))
	addSet("language", f.Languages)
	add("nsfw", string(f.NSFW))
	add("age_category", intPoint(f.AgeCategory))
	add("released_from", point(f.ReleasedFrom))
	add("released_to", point(f.ReleasedTo))
	add("rating_from", intPoint(f.RatingFrom))
	add("rating_to", intPoint(f.RatingTo))
	add("unrated", boolPoint(f.Unrated))
	add("favorite", boolPoint(f.Favorite))
	addSet("library", f.Libraries)
	add("has_copy", boolPoint(f.HasCopy))

	sort.Strings(parts)
	canonical := strings.Join(parts, "&") + "|" + string(f.Sort) + "|" + string(f.Direction)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])[:16]
}
