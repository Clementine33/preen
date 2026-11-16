package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/works"
)

// emptyRequest is an optional empty JSON request body.
type emptyRequest struct{}

// emptyEvent is an SSE event payload with no fields ({}).
type emptyEvent struct{}

// nullableInt is a three-state integer request field: omitted, null, or value.
type nullableInt struct {
	Set   bool
	Value *int
}

// UnmarshalJSON records explicit presence, distinguishing null from omission.
func (n *nullableInt) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Value = &value
	return nil
}

func (n nullableInt) toWorks() works.NullableInt {
	return works.NullableInt{Set: n.Set, Value: n.Value}
}

// nullableString is a three-state string request field: omitted, null, or value.
type nullableString struct {
	Set   bool
	Value *string
}

// UnmarshalJSON records explicit presence, distinguishing null from omission.
func (n *nullableString) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Value = &value
	return nil
}

func (n nullableString) toWorks() works.NullableString {
	return works.NullableString{Set: n.Set, Value: n.Value}
}

// orEmpty keeps a nil slice from encoding as null: response arrays are never
// null (docs/api/common.md#wire-schemas).
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// errorResponse is the standard API error envelope: {"code":"...","message":"..."}.
type errorResponse struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// writeError aborts the request with the given status and envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Code: code, Message: message})
}

// writeInternal answers 500 / INTERNAL with message and hands err to the
// access log, which writes it on this request's line (see failed).
func writeInternal(w http.ResponseWriter, err error, message string) {
	failed(w, err)
	writeError(w, http.StatusInternalServerError, "INTERNAL", message)
}

// failed records err as the cause the access log writes for this request: the
// client gets the stable envelope, the log gets the reason.
func failed(w http.ResponseWriter, err error) {
	for {
		switch rw := w.(type) {
		case *responseState:
			rw.cause = err
			return
		case interface{ Unwrap() http.ResponseWriter }:
			w = rw.Unwrap()
		default:
			return
		}
	}
}

// writeErrorDetails is writeError for a refusal that has something specific to
// say, such as which references still hold a work in place.
func writeErrorDetails(w http.ResponseWriter, status int, code, message string, details []string) {
	writeJSON(w, status, errorResponse{Code: code, Message: message, Details: details})
}

// healthResponse is the payload of GET /api/v1/health.
type healthResponse struct {
	OK                bool   `json:"ok"`
	Version           string `json:"version"`
	WorksetMaxMembers int    `json:"workset_max_members"`
}

// libraryResponse mirrors the repo Library type with snake_case JSON.
type libraryResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	RootPath       string     `json:"root_path"`
	StorageClass   string     `json:"storage_class"    jsonschema:"enum=hdd,enum=ssd"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastScanAt     *time.Time `json:"last_scan_at"`
	LastScanStatus string     `json:"last_scan_status"`
	LastScanError  string     `json:"last_scan_error"`
}

func toLibraryResponse(l *library.Library) libraryResponse {
	return libraryResponse{
		ID:             l.ID,
		Name:           l.Name,
		RootPath:       l.RootPath,
		StorageClass:   string(l.StorageClass),
		CreatedAt:      l.CreatedAt,
		UpdatedAt:      l.UpdatedAt,
		LastScanAt:     l.LastScanAt,
		LastScanStatus: l.LastScanStatus,
		LastScanError:  l.LastScanError,
	}
}

// libraryCreateRequest is the POST /api/v1/libraries payload.
type libraryCreateRequest struct {
	Name     string `json:"name"`
	RootPath string `json:"root_path"`
}

// libraryPatchRequest is the PATCH /api/v1/libraries/:id payload. Pointer
// fields distinguish absent fields from explicit empty values.
type libraryPatchRequest struct {
	Name         *string         `json:"name"`
	StorageClass json.RawMessage `json:"storage_class"`
}
