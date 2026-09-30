package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/onsei/organizer/backend/internal/thumbnail"
	"github.com/onsei/organizer/backend/internal/works"
)

// getWorkThumb serves one manifest image's thumbnail (ADR 0015 §3). The
// source goes through the same copy binding, containment and inventory
// whitelist as the file endpoint; the answer is immutable only at the image's
// current version, so a stale address never pins the wrong bytes.
func (s *Server) getWorkThumb(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	requested, _ := strconv.Atoi(r.URL.Query().Get("h"))
	height, ok := thumbnail.SnapHeight(requested)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "h must be a positive height in pixels")
		return
	}
	loc, refusal := service.ContentFile(code, r.PathValue("copy"), r.PathValue("path"))
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	if loc.Kind != works.KindImage {
		writeError(w, http.StatusNotFound, "NOT_AN_IMAGE", "the path is not an image of the manifest")
		return
	}

	etag := `"` + loc.ThumbV + "-" + strconv.Itoa(height) + `"`
	w.Header().Set("ETag", etag)
	if r.URL.Query().Get("v") == loc.ThumbV {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	img, err := s.deps.Thumbs.Get(r.Context(), thumbnail.Source{Path: loc.AbsPath, Version: loc.ThumbV}, height)
	switch {
	case err == nil:
	case errors.Is(err, thumbnail.ErrBusy):
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "BUSY", "thumbnail queue is full")
		return
	case errors.Is(err, thumbnail.ErrTooLarge):
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_TOO_LARGE", "the image exceeds the thumbnail pixel limit")
		return
	case errors.Is(err, thumbnail.ErrUndecodable):
		writeError(w, http.StatusUnprocessableEntity, "IMAGE_UNDECODABLE", "the image could not be decoded")
		return
	case r.Context().Err() != nil:
		return
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL", "the thumbnail could not be rendered")
		return
	}
	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(img.Bytes)))
	_, _ = w.Write(img.Bytes)
}
