package workset

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/library"
)

// Store is the storage the workset lifecycle needs: the operations its use
// cases call, no per-table repository and no read it does not make. The SQLite
// adapter satisfies it with its one concrete Repository.
//
// The composite writes stay whole here — replacing the current record,
// publishing a revision and retiring the plan it replaced, creating an
// execution session under its guards, syncing observed inventory beside the
// credentials of what was generated, and pushing one deployment-wide common
// setting across every editable record — because each of them is one
// atomic publication or guarded workflow whose invariants the caller cannot
// re-establish from outside. Revision payloads stage in bounded transactions.
type Store interface {
	HasActiveGenerationForRoot(string) (bool, error)
	HasActiveExecutionForRootKinds(string, []string) (bool, error)
	// CancelExecution is one operation of the lifecycle.
	CancelExecution(executionID string) error
	// CancelGeneration is one operation of the lifecycle.
	CancelGeneration(generationID string) error
	// ClearExpiredWorksetIdemKey is one operation of the lifecycle.
	ClearExpiredWorksetIdemKey(id string, cutoff time.Time) error
	// CompleteGenerationCanceled is one operation of the lifecycle.
	CompleteGenerationCanceled(generationID string) error
	MarkGenerationInterrupted(generationID string) error
	// CreateExecutionGuarded is one operation of the lifecycle.
	CreateExecutionGuarded(e *PlanExecution, g ExecutionGuards) error
	// CreateGeneration is one operation of the lifecycle.
	CreateGeneration(g *PlanGeneration) error
	// DirAudioCounts is one operation of the lifecycle.
	DirAudioCounts(rootPath string, relPaths []string) (map[string]int, error)
	ListLibraryWorkRoots(rootPath string) ([]string, error)
	// FinishExecution is one operation of the lifecycle.
	FinishExecution(executionID, status, errorCode, errorMessage string) error
	// GetActiveExecutionForOperation is one operation of the lifecycle.
	GetActiveExecutionForOperation(worksetID, operationType string) (*PlanExecution, error)
	// GetActiveGenerationForOperation is one operation of the lifecycle.
	GetActiveGenerationForOperation(worksetID, operationType string) (*PlanGeneration, error)
	// GetCommonSettings is one operation of the lifecycle.
	GetCommonSettings(operationType string) (*CommonSettings, error)
	// GetCurrentWorkset is one operation of the lifecycle.
	GetCurrentWorkset(libraryID string) (*Workset, error)
	// GetExecution is one operation of the lifecycle.
	GetExecution(executionID string) (*PlanExecution, error)
	// GetExecutionByOperationKey is one operation of the lifecycle.
	GetExecutionByOperationKey(worksetID, operationType, key string) (*PlanExecution, error)
	// GetExecutionForRevision is one operation of the lifecycle.
	GetExecutionForRevision(planID string) (*PlanExecution, error)
	// GetExecutionPosition is one operation of the lifecycle.
	GetExecutionProgress(executionID string) (*PlanExecution, error)
	// GetGeneration is one operation of the lifecycle.
	GetGeneration(generationID string) (*PlanGeneration, error)
	// GetGenerationByOperationKey is one operation of the lifecycle.
	GetGenerationByOperationKey(worksetID, operationType, key string) (*PlanGeneration, error)
	// GetLibrary is one operation of the lifecycle.
	GetLibrary(id string) (*library.Library, error)
	// GetOperation is one operation of the lifecycle.
	GetOperation(worksetID, operationType string) (*Operation, error)
	// GetOperationDraft is one operation of the lifecycle.
	GetOperationDraft(worksetID, operationType string) (*OperationDraft, error)
	// GetOperationRevision is one operation of the lifecycle.
	GetOperationRevision(worksetID, operationType, planID string) (*OperationRevision, error)
	// GetPlanComponentOutcomes reads only the requested frozen component payloads.
	GetPlanComponentOutcomes(planID string, indexes []int) (map[int]json.RawMessage, error)
	// GetPlanDetail is one operation of the lifecycle.
	GetPlanDetail(planID string) (*PlanDetail, error)
	// GetMemberPlanDetail reads only the first frozen root at rootPath (root_index
	// order), matching member conclusion ownership, and only that root's outcomes.
	GetMemberPlanDetail(planID, rootPath string) (*PlanDetail, error)
	// GetWorkset is one operation of the lifecycle.
	GetWorkset(id string) (*Workset, error)
	// GetWorksetByCreationIdemKey is one operation of the lifecycle.
	GetWorksetByCreationIdemKey(key string) (*Workset, error)
	// HasActiveScanForRoot is one operation of the lifecycle.
	HasActiveScanForRoot(rootPath string) (bool, error)
	// LatestExecutionForOperation is one operation of the lifecycle.
	LatestExecutionForOperation(worksetID, operationType string) (*PlanExecution, error)
	// LatestGenerationForOperation is one operation of the lifecycle.
	LatestGenerationForOperation(worksetID, operationType string) (*PlanGeneration, error)
	// ListExecutionComponentResults is one operation of the lifecycle.
	ListExecutionComponentResults(executionID string, fromIndex, limit int) ([]ExecutionComponentResult, error)
	// ListOperations is one operation of the lifecycle.
	ListOperations(worksetID string) ([]*Operation, error)
	// ListWorksetMembers is one operation of the lifecycle.
	ListWorksetMembers(worksetID string) ([]*WorksetMember, error)
	// ListWorksets is one operation of the lifecycle.
	ListWorksets(
		cursorUpdatedAt string,
		cursorID string,
		limit int,
		libraryID string,
		includeOrphaned bool,
	) ([]*Workset, error)
	// MarkGenerationFailed is one operation of the lifecycle.
	MarkGenerationFailed(generationID, code, message string) error
	// NextQueuedExecution is one operation of the lifecycle.
	NextQueuedExecution() (*PlanExecution, error)
	// NextQueuedGeneration claims FIFO among kinds; nil allows all, empty allows none.
	NextQueuedGeneration(kinds []string) (*PlanGeneration, error)
	// PersistOperationRevision stages guarded batches, checking ctx between them,
	// then atomically publishes the revision and completes its running generation.
	PersistOperationRevision(
		ctx context.Context,
		genID, worksetID, operationType string,
		now time.Time,
		p OperationRevisionPersist,
	) error
	// RenameWorkset is one operation of the lifecycle.
	RenameWorkset(id, title string, expectedVersion int, now time.Time) error
	// ReplaceCurrentWorkset is one operation of the lifecycle.
	ReplaceCurrentWorkset(
		w *Workset,
		members []WorksetMember,
		operations []Operation,
		drafts []OperationDraft,
		expectedCurrentID string,
	) error
	// SaveExecutionComponentResult is one operation of the lifecycle.
	SaveExecutionComponentResult(executionID string, res ExecutionComponentResult, p ExecutionPosition) error
	// SaveOperationDraft is one operation of the lifecycle.
	SaveOperationDraft(
		worksetID, operationType string,
		schemaVersion int,
		draftJSON, draftHash string,
		expectedVersion int,
		now time.Time,
	) error
	// SyncCommonSettings is one operation of the lifecycle.
	SyncCommonSettings(p CommonSettingsSync) error
	// SyncObservedInventory is one operation of the lifecycle.
	SyncObservedInventory(
		rootPath string,
		removed []string,
		changed []inventory.InventoryFile,
		generated []inventory.GenerationRecord,
	) error
	// UpdateExecutionPosition is one operation of the lifecycle.
	UpdateExecutionPosition(executionID string, p ExecutionPosition) error
	// UpdateGenerationProgress is one operation of the lifecycle.
	UpdateGenerationProgress(generationID string, completedRoots, errorCount int, currentRoot string) error
}
