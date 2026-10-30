package workset

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// OperationTypeConversion is the only publicly available operation type of
// this iteration. Encode/delete filesystem actions are a different concept and
// never appear here (ADR 0001).
const OperationTypeConversion = "conversion"

// Operation planning states (derived per operation, never stored).
const (
	PlanningUnplanned     = "unplanned"
	PlanningPlanned       = "planned"
	PlanningNeedsPlanning = "needs_planning"
	PlanningPlanning      = "planning"
	PlanningOrphaned      = "orphaned"
)

// Revision validation state values (revision-level, derived on read).
const (
	ValidationValid       = "valid"
	ValidationStale       = "stale"
	ValidationUnavailable = "unavailable"
)

// Error kinds for the workset usecase error envelope.
const (
	ErrKindInvalidArgument = "invalid_argument"
	ErrKindNotFound        = "not_found"
	ErrKindConflict        = "conflict"
	ErrKindPrecondition    = "precondition_failed"
	ErrKindInternal        = "internal"
)

// Error is the workset usecase error with a stable machine code. Details
// carries machine-readable reason codes (e.g. PLAN_NOT_CONFIRMABLE reasons);
// Stage names the unit stage a failure happened in, when the failure belongs
// to one execution unit's own work.
type Error struct {
	Kind    string
	Code    string
	Message string
	Stage   string
	Details []string
	Cause   error
}

// NewError creates a workset usecase error.
func NewError(kind, code, message string, cause error) *Error {
	return &Error{Kind: kind, Code: code, Message: message, Cause: cause}
}

// WithStage names the unit stage the failure happened in.
func (e *Error) WithStage(stage string) *Error {
	e.Stage = stage
	return e
}

// WithDetails attaches machine-readable reason codes to the error.
func (e *Error) WithDetails(details []string) *Error {
	e.Details = details
	return e
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Kind + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Kind + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// AsError extracts a *Error from an error chain.
func AsError(err error) (*Error, bool) {
	if e, ok := errors.AsType[*Error](err); ok {
		return e, true
	}
	return nil, false
}

// LibraryRef is the owning-library snapshot exposed on the workset view.
type LibraryRef struct {
	LibraryID string
	Name      string
	RootPath  string
}

// MemberView is one member directory. MemberID is the stable identity
// persisted on the member row and RelPath is the durable library-relative
// path; participation and overrides belong to an operation, not to the member.
type MemberView struct {
	MemberID   string
	FolderPath string
	FolderName string
	RelPath    string
}

// RevisionCounts are independent plan facts (ADR 0001): a component with
// changes, an unmet target, a blocked component and an unchanged component are
// separate counts that may overlap. No exclusive status label is derived.
type RevisionCounts struct {
	Members      int `json:"members"`
	Changed      int `json:"changed"`
	UnmetTargets int `json:"unmet_targets"`
	Blocked      int `json:"blocked"`
	Unchanged    int `json:"unchanged"`
}

// RevisionSummary is the compact immutable conclusion of one operation
// revision.
type RevisionSummary struct {
	PlanID          string
	RevisionIndex   int
	CreatedAt       time.Time
	Status          string
	SummaryReason   string
	Counts          RevisionCounts
	ValidationState string // valid | stale | unavailable
	Stale           *bool  // nil when validation_state == unavailable
}

// GenerationProgress is root-level progress of an active session.
type GenerationProgress struct {
	GenerationID   string
	Status         string
	TotalRoots     int
	CompletedRoots int
	CurrentRoot    string
	ErrorCount     int
}

// GenerationSummary is the compact terminal summary of a session.
type GenerationSummary struct {
	GenerationID string
	Status       string
	ErrorCode    string
	ErrorMessage string
	FinishedAt   time.Time
}

// OperationView is the operation-scoped aggregate. It is the state the
// workbench reads for one operation; nothing here is workset-wide.
type OperationView struct {
	WorksetID        string
	OperationType    string
	Version          int
	PlanningState    string
	CurrentRevision  *RevisionSummary
	ActiveGeneration *GenerationProgress
	LatestGeneration *GenerationSummary
	// ActiveExecution is the queued/running execution session of this
	// operation, if any; it blocks generation, execution and draft edits.
	ActiveExecution *ExecutionProgress
	// LatestExecution is the newest execution session (any status) so the UI can
	// reach a finished partial run after a reload.
	LatestExecution *ExecutionRef
}

// WorksetView is the workset metadata view: identity, library, fixed members
// and timestamps. Planning state is read from the operation view.
type WorksetView struct {
	WorksetID string
	Title     string
	Version   int
	Library   *LibraryRef
	Members   []MemberView
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateCurrentRequest is the create-or-replace payload of the current record
// of one library, shared by all tasks. FolderPaths are library-relative member
// directory paths — never scan-scoped ids — and ExpectedCurrentID is the
// record the caller saw as current ("" when it saw none), which is what makes
// a concurrent replace a conflict instead of an overwrite.
type CreateCurrentRequest struct {
	LibraryID         string
	OperationType     string // task entry point only; membership is shared
	Title             string
	FolderPaths       []string
	ExpectedCurrentID string
	IdempotencyKey    string
}

// Skipped folder reasons: a selected directory that did not become a member,
// reported so the created scope is never silently wider or narrower than what
// the caller asked for.
const (
	SkipNoAudio       = "no_audio"
	SkipMissing       = "missing"
	SkipRecoveryDir   = "recovery_dir"
	SkipDuplicate     = "duplicate"
	SkipInvalidPath   = "invalid_path"
	SkipNotWorkRoot   = "not_work_root"
	SkipOutsideMember = "outside_member"
)

// SkippedFolder is one selected directory left out of the record.
type SkippedFolder struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// CreateCurrentResult distinguishes a fresh creation from an idempotent
// replay, and reports the scope that was actually recorded.
type CreateCurrentResult struct {
	Workset *WorksetView
	Created bool
	// Recorded is the number of members actually written.
	Recorded int
	// Skipped lists the selected directories that were left out, with why.
	Skipped []SkippedFolder
}

// ListQuery is the workset list input.
type ListQuery struct {
	Cursor          string // "<updated_at RFC3339Nano>_<workset_id>"
	Limit           int
	LibraryID       string
	IncludeOrphaned bool
}

// RenameRequest is the PATCH /worksets/{id} payload.
type RenameRequest struct {
	Title          string
	IfMatchVersion int
}

// Draft is the persisted sparse operation draft. Version is the operation
// version (the If-Match authority); there is no separate draft counter. The
// document is the task's opaque payload.
type Draft struct {
	WorksetID     string
	OperationType string
	Version       int
	SchemaVersion int
	Document      json.RawMessage
	UpdatedAt     time.Time
}

// SaveDraftRequest is the PUT operation draft payload (full replacement of the
// sparse document, never a materialized full-config document). The document is
// the task's opaque payload.
type SaveDraftRequest struct {
	Document       json.RawMessage
	IfMatchVersion int
}

// StartGenerationRequest is the POST operation revisions payload.
type StartGenerationRequest struct {
	IfMatchVersion int
	IdempotencyKey string
}

// StartGenerationResult distinguishes a fresh session from a replay/dedup.
type StartGenerationResult struct {
	Generation *GenerationView
	Revision   *RevisionSummary // present for created:false unchanged-input replays
	Created    bool
}

// GenerationView is the session detail payload.
type GenerationView struct {
	GenerationID   string
	WorksetID      string
	OperationType  string
	Status         string
	TotalRoots     int
	CompletedRoots int
	CurrentRoot    string
	ErrorCount     int
	RevisionID     string
	ErrorCode      string
	ErrorMessage   string
	StartedAt      time.Time
	FinishedAt     time.Time
	CreatedAt      time.Time
}

// RevisionMember is one member of a frozen revision: its effective config as
// the task's opaque payload, where each unit came from, and whether it
// participated. It is derived from the frozen sparse draft, never from the
// live one.
type RevisionMember struct {
	MemberID   string
	FolderPath string
	MemberName string
	Excluded   bool
	Payload    json.RawMessage // task payload: the effective config
	Sources    map[string]string
	Conclusion MemberConclusion
}

// MemberConclusion combines workset input facts with the task's opaque conclusion.
// Input validation may change while the frozen plan and task facts stay fixed.
type MemberConclusion struct {
	MemberID      string          `json:"member_id"`
	Excluded      bool            `json:"excluded"`
	Input         string          `json:"input"`
	HasComponents bool            `json:"has_components"`
	Task          json.RawMessage `json:"task,omitempty"`
}

// RevisionPlan is the reviewable plan snapshot of one revision, rebuilt from
// the persisted records (never from live policy state). A revision carries
// exactly one plan; the persisted step, root and component rows flatten into
// it, and the payload decode belongs to the operation's task.
type RevisionPlan struct {
	PlanID            string
	SnapshotToken     string
	RootPath          string
	TaskKind          string
	TaskSchemaVersion int
	Status            string
	PolicyHash        string
	Summary           PlanSummary
	Payload           json.RawMessage // task plan payload (policy JSON)
	ClassifierTags    []string
	ClassifierHash    string
	StepSummary       json.RawMessage   // the task's payload summary, as stored
	Units             []json.RawMessage // per-unit payloads in component order
}

// RevisionView is the lightweight current plan detail.
type RevisionView struct {
	PlanID        string
	RevisionIndex int
	CreatedAt     time.Time
	Plan          RevisionPlan
	// Members contains live input facts and task conclusions for the frozen plan.
	Members []MemberConclusion
	// Counts are the independent plan facts of this revision.
	Counts RevisionCounts
	// Execution is the newest execution session of this revision (any status).
	// A revision is executed at most once, so a present session also means the
	// revision can no longer be executed again.
	Execution *ExecutionRef
}

// MemberReview is one member's frozen settings and owned plan inputs.
type MemberReview struct {
	PlanID string
	Member RevisionMember
	Roots  []RootValidation
	Plan   RevisionPlan
}

// RootValidation is per-root validation of a revision snapshot.
type RootValidation struct {
	RootIndex            int
	RootPath             string
	RootStatus           string
	RootErrorCode        string
	RootErrorMessage     string
	Stale                bool
	InventoryFingerprint string
	EntryCount           int
}

// Dispatcher is the background FIFO generation scheduler handle for main
// wiring (Start once at process startup, Stop at graceful shutdown).
type Dispatcher interface {
	Start()
	Stop(context.Context) error
}

// Service is the workset usecase contract. Every operation-scoped method takes
// the workset id and the operation type: no mutable state is addressable by
// workset id alone.
type Service interface {
	// SetExclusiveEnqueueGuard wires scan-exclusive session admission before workers start.
	SetExclusiveEnqueueGuard(EnqueueGuard)
	DispatcherHandle() Dispatcher
	CreateCurrentWorkset(ctx context.Context, req CreateCurrentRequest) (*CreateCurrentResult, error)
	GetCurrentWorkset(ctx context.Context, libraryID, operationType string) (*WorksetView, error)
	ListWorksets(ctx context.Context, q ListQuery) ([]*WorksetView, string, error)
	GetWorkset(ctx context.Context, id string) (*WorksetView, error)
	RenameWorkset(ctx context.Context, id string, req RenameRequest) (*WorksetView, error)
	GetOperation(ctx context.Context, worksetID, operationType string) (*OperationView, error)
	GetDraft(ctx context.Context, worksetID, operationType string) (*Draft, error)
	SaveDraft(ctx context.Context, worksetID, operationType string, req SaveDraftRequest) (*OperationView, error)
	GetTaskSettings(ctx context.Context, operationType string) (*TaskSettings, error)
	SaveTaskSettings(
		ctx context.Context,
		operationType string,
		payload json.RawMessage,
		ifMatch string,
	) (*TaskSettings, error)
	StartGeneration(
		ctx context.Context,
		worksetID, operationType string,
		req StartGenerationRequest,
	) (*StartGenerationResult, error)
	GetGeneration(ctx context.Context, worksetID, operationType, generationID string) (*GenerationView, error)
	CancelGeneration(ctx context.Context, worksetID, operationType, generationID string) (*GenerationView, error)
	Subscribe(
		ctx context.Context,
		worksetID, operationType, generationID string,
		emit func(event string, data any) error,
	) error
	GetRevision(ctx context.Context, worksetID, operationType, planID string) (*RevisionView, error)
	GetMemberReview(ctx context.Context, worksetID, operationType, planID, memberID string) (*MemberReview, error)
	StartExecution(
		ctx context.Context,
		worksetID, operationType, planID string,
		req StartExecutionRequest,
	) (*StartExecutionResult, error)
	GetExecution(
		ctx context.Context,
		worksetID, operationType, executionID string,
		read ExecutionRead,
	) (*ExecutionView, error)
	CancelExecution(
		ctx context.Context,
		worksetID, operationType, executionID string,
	) (*ExecutionView, error)
	SubscribeExecution(
		ctx context.Context,
		worksetID, operationType, executionID string,
		emit func(event string, data any) error,
	) error
	// SubscribeExecutionMembers is SubscribeExecution in the members view.
	SubscribeExecutionMembers(
		ctx context.Context,
		worksetID, operationType, executionID string,
		emit func(event string, data any) error,
	) error
}

// toGenerationView converts a persisted session row to the view payload.
func toGenerationView(g *PlanGeneration) *GenerationView {
	return &GenerationView{
		GenerationID:   g.GenerationID,
		WorksetID:      g.WorksetID,
		OperationType:  g.OperationType,
		Status:         g.Status,
		TotalRoots:     g.TotalRoots,
		CompletedRoots: g.CompletedRoots,
		CurrentRoot:    g.CurrentRoot,
		ErrorCount:     g.ErrorCount,
		RevisionID:     g.RevisionID,
		ErrorCode:      g.ErrorCode,
		ErrorMessage:   g.ErrorMessage,
		StartedAt:      g.StartedAt,
		FinishedAt:     g.FinishedAt,
		CreatedAt:      g.CreatedAt,
	}
}
