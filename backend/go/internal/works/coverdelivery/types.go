package coverdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const MaxBytes = 16 << 20

// Error is a stable cover failure code, independent of metadata execution.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string             { return e.Code + ": " + e.Message }
func Failure(code string, err error) error { return &Error{Code: code, Message: err.Error()} }

// Validators are sent only when a matching local file is still available.
type Validators struct {
	ETag         string
	LastModified string
}
type Download struct {
	Data         []byte
	Extension    string
	ContentType  string
	ETag         string
	LastModified string
	NotModified  bool
}
type Downloader interface {
	Download(context.Context, string, Validators) (Download, error)
}

// Target identifies a binding, never an arbitrary caller-supplied file path.
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

// Job is one shared download, with independently cancelable delivery requests.
type Job struct {
	MainChecked   bool
	BackupChecked bool
	ID            string
	WorkCode      string
	URL           string
	BackupURL     string
	Locale        string
	Refresh       bool
	Version       int64
	StagePath     string
	Extension     string
	Digest        string
	ETag          string
	LastModified  string
}
type Request struct {
	MainChecked   bool
	BackupChecked bool
	ExecutionID   string
	WorkCode      string
	URL           string
	BackupURL     string
	Locale        string
	Refresh       bool
	Targets       []Target
}
type Report struct {
	Items     []Destination  `json:"items"`
	Counts    map[string]int `json:"counts"`
	Downloads map[string]int `json:"downloads"`
}
type Store interface {
	EnqueueCovers(context.Context, Request) error
	ClaimCover(context.Context) (*Job, error)
	SaveCoverStage(context.Context, Job) error
	CoverDestinations(context.Context, string) ([]Destination, error)
	SaveCoverDestination(context.Context, Destination) error
	CoverCurrent(context.Context, Job) (bool, error)
	FinishCover(context.Context, string) error
	RecoverCovers(context.Context) error
	ActiveCoverJobs(context.Context) (map[string]bool, error)
	CoverReport(context.Context, string) (Report, error)
	ChangeCoverRequests(context.Context, string, bool) error
	StoredCover(context.Context, string) (*Stored, error)
	SaveStoredCover(context.Context, string, Stored) error
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

func changed() error { return Failure("INPUT_CHANGED", fmt.Errorf("copy identity or binding changed")) }

// Existing is a validated on-disk main image.
type Existing struct {
	Name          string
	Data          []byte
	Extension     string
	Digest        string
	NeedsRecovery bool
}
type Publication struct {
	Name    string
	Digest  string
	Skipped bool
}
type Files interface {
	Existing(Target) (*Existing, error)
	Publish(context.Context, Target, []byte, bool) (Publication, error)
}

// Staging retains validated download bytes across delivery attempts. Saved paths
// are opaque to the workflow; the adapter checks them against the job's location.
type Staging interface {
	Read(jobID, savedPath string) ([]byte, error)
	Write(jobID string, data []byte) (string, error)
	Remove(jobID, savedPath string) error
	Cleanup(activeJobs map[string]bool) error
}

type Stored struct {
	URL          string
	Name         string
	Digest       string
	ETag         string
	LastModified string
}
