package httpapi

import (
	"net/http"

	"github.com/Clementine33/preen/backend/internal/works"
)

// randomWorkResponse is the GET /api/v1/works/random payload: the chosen work
// and its 0-based position in the ordinary listing under the same parameters.
type randomWorkResponse struct {
	RJ    string `json:"rj"`
	Index int    `json:"index"`
}

// randomWork picks one work of the current listing, preferring unheard ones.
// See docs/api/works.md#random-work.
func (s *Server) randomWork(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	filter, exclude, refusal := randomWorkParams(r)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	pick, found, refusal := service.Random(filter, exclude)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	if !found {
		writeWorksError(w, works.NotFound(works.CodeWorkNotFound, "no work matches the filter"))
		return
	}
	writeJSON(w, http.StatusOK, randomWorkResponse{RJ: string(pick.ID), Index: pick.Index})
}

// randomWorkParams reads the listing's filter and order plus `exclude`. The
// page window describes a page rather than the set, so it is refused the way
// the facet lists refuse it; `exclude` is taken out before the listing's own
// parser, which would reject it as unknown.
func randomWorkParams(r *http.Request) (works.Filter, works.WorkCode, *works.Error) {
	query := r.URL.Query()
	for _, name := range []string{"limit", "page"} {
		if query.Has(name) {
			return works.Filter{}, "", works.Invalid(works.CodeInvalidArgument, "unknown query parameter: "+name)
		}
	}
	var exclude works.WorkCode
	if raw := query.Get("exclude"); raw != "" {
		code, ok := works.ParseWorkCode(raw)
		if !ok {
			return works.Filter{}, "", works.Invalid(works.CodeInvalidArgument, "exclude must be a work number")
		}
		exclude = code
	}
	query.Del("exclude")
	stripped := *r.URL
	stripped.RawQuery = query.Encode()
	clone := *r
	clone.URL = &stripped
	filter, refusal := parseWorkFilter(&clone)
	return filter, exclude, refusal
}
