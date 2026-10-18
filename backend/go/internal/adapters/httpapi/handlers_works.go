package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Clementine33/preen/backend/internal/works"
)

// writeWorksError maps the catalog's own refusal to its response.
func writeWorksError(w http.ResponseWriter, err *works.Error) {
	status := http.StatusInternalServerError
	switch err.Kind {
	case works.KindInvalid:
		status = http.StatusBadRequest
	case works.KindNotFound:
		status = http.StatusNotFound
	case works.KindConflict:
		status = http.StatusConflict
	case works.KindInternal:
		status = http.StatusInternalServerError
	}
	writeErrorDetails(w, status, err.Code, err.Message, err.Details)
}

// workPath resolves the work number in the request path.
func (s *Server) workPath(w http.ResponseWriter, r *http.Request) (works.WorkCode, bool) {
	code, ok := works.ParseWorkCode(r.PathValue("rj"))
	if !ok {
		writeError(w, http.StatusBadRequest, works.CodeInvalidArgument, "the path must name a work number")
		return "", false
	}
	return code, true
}

// catalog answers whether the catalog entry is wired.
func (s *Server) catalog(w http.ResponseWriter) (*works.Service, bool) {
	if s.deps.Works == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "the work catalog is not configured")
		return nil, false
	}
	return s.deps.Works, true
}

// ==================== listing ====================

// userTagResponse is one user tag.
type userTagResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func toUserTagResponses(tags []works.UserTag) []userTagResponse {
	out := make([]userTagResponse, 0, len(tags))
	for _, tag := range tags {
		out = append(out, userTagResponse{ID: tag.ID, Name: tag.Name})
	}
	return out
}

// workSummaryResponse is one row of the ordinary listing.
type workSummaryResponse struct {
	RJ           string            `json:"rj"`
	Title        *string           `json:"title"`
	ThumbnailURL *string           `json:"thumbnail_url"`
	Circle       *string           `json:"circle"`
	Language     *string           `json:"language"`
	NSFW         *bool             `json:"nsfw"`
	AgeCategory  *int              `json:"age_category"`
	ReleaseDate  *string           `json:"release_date"`
	DLCount      *int              `json:"dl_count"`
	Rating       *int              `json:"rating"`
	Favorite     bool              `json:"favorite"`
	UserTags     []userTagResponse `json:"user_tags"`
	HasNote      bool              `json:"has_note"`
	CollectedAt  string            `json:"collected_at"`
	CopyCount    int               `json:"copy_count"`
	// LocalCover is the copy's cover already on disk; the bytes are served by
	// the content route.
	LocalCover *localCoverResponse `json:"local_cover"`
}

type localCoverResponse struct {
	CopyID  string `json:"copy_id"`
	RelPath string `json:"rel_path"`
	Version string `json:"version"`
}

// attachLocalCovers adds the page's covers with one lookup. A failed lookup
// leaves the covers off: the remote thumbnail still answers.
func (s *Server) attachLocalCovers(r *http.Request, rows ...*workSummaryResponse) {
	if s.deps.LocalCovers == nil || len(rows) == 0 {
		return
	}
	ids := make([]works.WorkCode, len(rows))
	for i, row := range rows {
		ids[i] = works.WorkCode(row.RJ)
	}
	covers, err := s.deps.LocalCovers.LocalCovers(r.Context(), ids)
	if err != nil {
		return
	}
	for _, row := range rows {
		if c, ok := covers[works.WorkCode(row.RJ)]; ok {
			row.LocalCover = &localCoverResponse{CopyID: c.CopyID, RelPath: c.RelPath, Version: c.Version}
		}
	}
}

func toWorkSummary(summary works.WorkSummary) workSummaryResponse {
	return workSummaryResponse{
		RJ:           string(summary.ID),
		Title:        summary.Title,
		ThumbnailURL: summary.ThumbnailURL,
		Circle:       summary.CircleName,
		Language:     summary.Language,
		NSFW:         summary.NSFW,
		AgeCategory:  summary.AgeCategory,
		ReleaseDate:  summary.ReleaseDate,
		DLCount:      summary.DLCount,
		Rating:       summary.Rating,
		Favorite:     summary.Favorite,
		UserTags:     toUserTagResponses(summary.UserTags),
		HasNote:      summary.HasNote,
		CollectedAt:  summary.CollectedAt,
		CopyCount:    summary.CopyCount,
	}
}

// listWorks answers one page of the ordinary listing and the listing's total.
func (s *Server) listWorks(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	filter, refusal := parseWorkFilter(r)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	page, refusal := service.List(filter)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	out := struct {
		Works []workSummaryResponse `json:"works"`
		Total int                   `json:"total"`
	}{Works: make([]workSummaryResponse, 0, len(page.Works)), Total: page.Total}
	for _, summary := range page.Works {
		out.Works = append(out.Works, toWorkSummary(summary))
	}
	rows := make([]*workSummaryResponse, len(out.Works))
	for i := range out.Works {
		rows[i] = &out.Works[i]
	}
	s.attachLocalCovers(r, rows...)
	writeJSON(w, http.StatusOK, out)
}

// filterKeys is the whole accepted query surface of the listing. An unknown
// parameter is refused rather than ignored, so a typo cannot silently return a
// wider answer than the caller asked for.
var filterKeys = map[string]bool{
	"q": true, "circle": true,
	"voice_actor": true, "voice_actor_match": true,
	"source_tag": true, "source_tag_match": true,
	"user_tag": true, "user_tag_match": true,
	"tag": true, "tag_match": true,
	"language": true, "nsfw": true, "age_category": true,
	"released_from": true, "released_to": true,
	"rating_from": true, "rating_to": true, "unrated": true,
	"favorite": true, "library": true, "has_copy": true,
	"sort": true, "direction": true, "limit": true, "page": true,
}

// parseWorkFilter reads the listing's query parameters. Every refusal is a 400
// with a stable code; nothing here substitutes a default for a value the caller
// actually supplied.
func parseWorkFilter(r *http.Request) (works.Filter, *works.Error) {
	query := r.URL.Query()
	var filter works.Filter
	for key := range query {
		if !filterKeys[key] {
			return filter, works.Invalid(works.CodeInvalidArgument, "unknown query parameter: "+key)
		}
	}
	first := func(key string) *string {
		values, present := query[key]
		if !present || len(values) == 0 || values[0] == "" {
			return nil
		}
		value := values[0]
		return &value
	}
	set := func(key string) []string {
		value := first(key)
		if value == nil {
			return nil
		}
		var out []string
		for part := range strings.SplitSeq(*value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	}

	filter.Query = first("q")
	filter.Circles = set("circle")
	filter.VoiceActors = set("voice_actor")
	filter.SourceTags = set("source_tag")
	filter.UserTags = set("user_tag")
	if value := first("tag"); value != nil {
		groups, refusal := works.ParseTagGroups(*value)
		if refusal != nil {
			return filter, refusal
		}
		filter.TagGroups = groups
	}
	filter.Languages = set("language")
	filter.Libraries = set("library")

	modes := []struct {
		key  string
		into *works.MatchMode
	}{
		{"voice_actor_match", &filter.VoiceActorMatch},
		{"source_tag_match", &filter.SourceTagMatch},
		{"user_tag_match", &filter.UserTagMatch},
		{"tag_match", &filter.TagMatch},
	}
	for _, mode := range modes {
		if value := first(mode.key); value != nil {
			*mode.into = works.MatchMode(*value)
		}
	}

	if value := first("nsfw"); value != nil {
		filter.NSFW = works.NSFWFilter(*value)
	}
	filter.ReleasedFrom = first("released_from")
	filter.ReleasedTo = first("released_to")
	if value := first("sort"); value != nil {
		filter.Sort = works.SortField(*value)
	}
	if value := first("direction"); value != nil {
		filter.Direction = works.Direction(*value)
	}

	if refusal := parseIntAndFlagFilters(query, first, &filter); refusal != nil {
		return filter, refusal
	}

	limit, refusal := parseLimit(r)
	if refusal != nil {
		return filter, refusal
	}
	filter.Limit = limit
	page, refusal := parsePage(r)
	if refusal != nil {
		return filter, refusal
	}
	filter.Page = page
	return filter, nil
}

// parseIntAndFlagFilters fills the numeric and boolean dimensions. A value that
// does not parse is refused rather than dropped: a caller who asked for a rating
// range and got an unconstrained listing has been answered a different question.
func parseIntAndFlagFilters(query url.Values, first func(string) *string, filter *works.Filter) *works.Error {
	for _, integer := range []struct {
		key  string
		into **int
	}{{"rating_from", &filter.RatingFrom}, {"rating_to", &filter.RatingTo}, {"age_category", &filter.AgeCategory}} {
		value := first(integer.key)
		if value == nil {
			continue
		}
		parsed, parseErr := strconv.Atoi(*value)
		if parseErr != nil {
			return works.Invalid(works.CodeInvalidArgument, integer.key+" must be an integer")
		}
		*integer.into = &parsed
	}
	for _, flag := range []struct {
		key  string
		into **bool
	}{{"unrated", &filter.Unrated}, {"favorite", &filter.Favorite}, {"has_copy", &filter.HasCopy}} {
		value := first(flag.key)
		if value == nil {
			continue
		}
		parsed, parseErr := strconv.ParseBool(*value)
		if parseErr != nil {
			return works.Invalid(works.CodeInvalidArgument, flag.key+" must be true or false")
		}
		*flag.into = &parsed
	}
	return nil
}

// parsePage reads the 1-based page number. An absent page is the first one;
// a supplied one must be a whole number of at least 1, so `page=0` is refused
// rather than read as "absent".
func parsePage(r *http.Request) (int, *works.Error) {
	values, present := r.URL.Query()["page"]
	if !present || len(values) == 0 || values[0] == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(values[0])
	if err != nil || page < 1 {
		return 0, works.Invalid(works.CodeInvalidPage, "page must be an integer of at least 1")
	}
	return page, nil
}

// parseLimit reads the page size. An absent parameter takes the default, while
// a parameter the caller did supply has to be a usable number: quietly
// substituting the default for "abc", or clamping 201 down to 200, would answer
// a question nobody asked.
func parseLimit(r *http.Request) (int, *works.Error) {
	values, present := r.URL.Query()["limit"]
	if !present || len(values) == 0 || values[0] == "" {
		return works.DefaultLimit, nil
	}
	limit, err := strconv.Atoi(values[0])
	if err != nil {
		return 0, works.Invalid(works.CodeInvalidPage, "limit must be an integer")
	}
	if limit < 1 || limit > works.MaxLimit {
		return 0, works.Invalid(
			works.CodeInvalidPage,
			"limit must be between 1 and "+strconv.Itoa(works.MaxLimit),
		)
	}
	return limit, nil
}
