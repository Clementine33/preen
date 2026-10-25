package coverdelivery

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

const MaxBytes = 16 << 20

// Error is a stable cover failure code, independent of metadata execution.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string             { return e.Code + ": " + e.Message }
func Failure(code string, err error) error { return &Error{Code: code, Message: err.Error()} }

type Download struct {
	Data        []byte
	Extension   string
	ContentType string
}
type Downloader interface {
	Download(context.Context, string) (Download, error)
}

// Target freezes one selected copy and its path. Identity is a legacy payload
// field; delivery no longer compares physical directory identities or bindings.
type Target struct {
	CopyID      string `json:"copy_id"`
	WorkCode    string `json:"rj"`
	LibraryRoot string `json:"library_root"`
	RelPath     string `json:"rel_path"`
	Identity    string `json:"identity"`
}
type Destination struct {
	ID           string `json:"id"`
	ExecutionID  string `json:"execution_id"`
	Target       Target `json:"target"`
	Refresh      bool   `json:"refresh"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Published    string `json:"published,omitempty"`
	Digest       string `json:"digest,omitempty"`
}

// Job is the pending deliveries of one work, claimed together for one download.
type Job struct {
	WorkCode     string
	Locale       string
	Destinations []Destination
}
type Request struct {
	ExecutionID string
	WorkCode    string
	Locale      string
	Refresh     bool
	Targets     []Target
}
type Report struct {
	Items  []Destination  `json:"items"`
	Counts map[string]int `json:"counts"`
}
type Store interface {
	EnqueueCovers(context.Context, Request) error
	// ClaimCover marks the oldest pending delivery and every other pending
	// delivery of its work running; it returns nil when nothing is pending.
	ClaimCover(context.Context) (*Job, error)
	SaveCoverDestination(context.Context, Destination) error
	// CoverCurrent reports whether any of the deliveries is still running.
	CoverCurrent(context.Context, []string) (bool, error)
	// FinishCover requeues the claimed deliveries that did not reach an outcome.
	FinishCover(context.Context, []string) error
	// RecoverCovers requeues deliveries a previous process left running.
	RecoverCovers(context.Context) error
	CoverReport(context.Context, string) (Report, error)
	ChangeCoverRequests(context.Context, string, bool) error
	// WorkCoverURL is the work's current main cover URL, empty when unknown.
	WorkCoverURL(context.Context, string) (string, error)
	ResolveCoverCopy(context.Context, string) (Target, error)
	CoverTargets(context.Context, string, []string) (map[string]Target, error)
}
type Source interface {
	FetchMain(context.Context, string, string, string) (string, error)
}

func Code(err error) string {
	e := &Error{}
	if errors.As(err, &e) {
		return e.Code
	}
	return "COVER_FAILED"
}
func Encode(value any) string          { b, _ := json.Marshal(value); return string(b) }
func unavailable(message string) error { return &Error{Code: "COVER_UNAVAILABLE", Message: message} }

// Existing is a validated on-disk main image.
type Existing struct {
	Name          string
	Data          []byte
	Extension     string
	Digest        string
	NeedsRecovery bool
	File          inventory.InventoryFile
}
type Publication struct {
	Name    string
	Digest  string
	Skipped bool
	File    *inventory.InventoryFile
	Removed []string
}
type Files interface {
	Existing(Target) (*Existing, error)
	Publish(context.Context, Target, []byte, string, bool) (Publication, error)
	// CompletePublication releases the recovery journal after inventory sync.
	CompletePublication(Target) error
}
