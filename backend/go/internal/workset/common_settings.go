package workset

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Why one record refuses a global common-settings save. The save is refused as
// a whole — never a part of it — whatever the reason.
const (
	// RefusalGenerating and RefusalExecuting: the record holds a queued or
	// running planning session or execution, which froze the draft it is
	// working from.
	RefusalGenerating = "generating"
	RefusalExecuting  = "executing"
	// RefusalUnreadableDraft: a stored document the task cannot read, so its
	// common fields cannot be rewritten. Refusing beats guessing: leaving that
	// one record behind would be the partial sync this rule exists to prevent
	// (ADR 0008).
	RefusalUnreadableDraft = "unreadable_draft"
)

// CommonSettingsRefusalError names the operation that refused a global
// common-settings save, and why.
type CommonSettingsRefusalError struct {
	WorksetID string
	Title     string
	Reason    string
}

func (e *CommonSettingsRefusalError) Error() string {
	return "record " + e.WorksetID + " (" + e.Title + ") refused the common settings: " + e.Reason
}

// CommonSettings is the stored deployment-wide common settings of one
// operation kind: the task's opaque common payload (ADR 0008).
type CommonSettings struct {
	OperationType string
	Settings      string
	UpdatedAt     time.Time
}

// CommonSettingsSync is one global common-settings save. The store applies the
// whole payload as one transaction (ADR 0008): the target operation's own
// draft — the member edits the caller made included —, the stored payload, and
// the common fields of every other editable operation of the kind, each with
// its draft hash and operation version.
type CommonSettingsSync struct {
	OperationType string
	// TargetWorksetID and TargetVersion are the operation the save came through
	// and the If-Match version it echoed.
	TargetWorksetID string
	TargetVersion   int
	// SchemaVersion, Document and DraftHash are the target operation's new
	// canonical draft document, already normalized by the task.
	SchemaVersion int
	Document      string
	DraftHash     string
	// Settings is the new deployment-wide common-settings payload.
	Settings string
	// Rewrite returns one other operation's new canonical document and hash
	// with Settings applied to its stored one, preserving its member records. A
	// stored document it cannot read refuses the whole save with a
	// *CommonSettingsRefusalError.
	Rewrite func(document string) (newDocument, hash string, err error)
	Now     time.Time
}

// seedCommonSettings is the common-settings payload a newly created operation
// starts from.
//
// A stored deployment-wide value wins. Without one, replacing a record keeps
// that record's own settings; a different record's later member-only save is
// not evidence that its common values should win (ADR 0008).
func (s *serviceImpl) seedCommonSettings(
	task Task,
	libraryID, operationType, currentWorksetID string,
) (json.RawMessage, error) {
	stored, err := s.repo.GetCommonSettings(operationType)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load the common settings", err)
	}
	if stored != nil {
		return json.RawMessage(stored.Settings), nil
	}
	if currentWorksetID == "" {
		return nil, nil
	}
	draft, err := s.repo.GetOperationDraft(currentWorksetID, operationType)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load the replaced draft", err)
	}
	if draft == nil {
		current, currentErr := s.repo.GetCurrentWorkset(libraryID)
		if currentErr != nil {
			return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load the current record", currentErr)
		}
		if current == nil || current.ID != currentWorksetID {
			return nil, NewError(
				ErrKindConflict, "RECORD_REPLACED",
				"the current record changed while this request was in flight", nil,
			)
		}
		return nil, NewError(ErrKindConflict, "COMMON_SETTINGS_UNREADABLE", "the replaced record has no draft", nil)
	}
	common, projectErr := task.CommonSettings([]byte(draft.DraftJSON))
	if projectErr != nil {
		return nil, NewError(
			ErrKindConflict, "COMMON_SETTINGS_UNREADABLE",
			"the replaced record's settings cannot be read", projectErr,
		)
	}
	return common, nil
}

// commonSettingsOf reports whether one normalized save changes the operation's
// common settings, and returns the payload that becomes the deployment-wide
// value when it does.
//
// The comparison is against the operation's own stored document, so the answer
// is "did this save edit a common value": a save that only moves member
// records keeps the stored common settings, and the deployment, exactly as
// they were (ADR 0008). An unreadable stored draft refuses the save.
func (s *serviceImpl) commonSettingsOf(
	task Task,
	worksetID, operationType string,
	canonical []byte,
) (json.RawMessage, bool, error) {
	submitted, err := task.CommonSettings(canonical)
	if err != nil {
		return nil, false, err
	}
	stored, err := s.repo.GetOperationDraft(worksetID, operationType)
	if err != nil {
		return nil, false, NewError(ErrKindInternal, "INTERNAL", "failed to load draft", err)
	}
	if stored == nil {
		return nil, false, NewError(
			ErrKindConflict, "COMMON_SETTINGS_UNREADABLE",
			"record "+worksetID+" has no draft; the global settings were not saved", nil,
		)
	}
	storedCommon, err := task.CommonSettings([]byte(stored.DraftJSON))
	if err != nil {
		return nil, false, NewError(
			ErrKindConflict, "COMMON_SETTINGS_UNREADABLE",
			"record "+worksetID+" holds a draft that cannot be read; the global settings were not saved", err,
		)
	}
	return submitted, CanonicalJSONHash(submitted) != CanonicalJSONHash(storedCommon), nil
}

// saveGlobalDraft writes a save whose common settings changed: the stored
// deployment-wide payload, this operation's own draft and every other editable
// operation's common fields move in one transaction, or none of them do
// (ADR 0008). Every generated plan is left alone — a plan is a frozen
// snapshot, and the rewritten drafts are what makes them need regeneration.
func (s *serviceImpl) saveGlobalDraft(
	ctx context.Context,
	task Task,
	worksetID, operationType string,
	schemaVersion int,
	canonical []byte, hash string,
	settings json.RawMessage,
	ifMatchVersion int,
) (*OperationView, error) {
	syncErr := s.repo.SyncCommonSettings(CommonSettingsSync{
		OperationType:   operationType,
		TargetWorksetID: worksetID,
		TargetVersion:   ifMatchVersion,
		SchemaVersion:   schemaVersion,
		Document:        string(canonical),
		DraftHash:       hash,
		Settings:        string(settings),
		Now:             time.Now(),
		Rewrite: func(document string) (string, string, error) {
			rewritten, rewrittenHash, applyErr := task.ApplyCommonSettings(
				[]byte(document),
				settings,
			)
			if applyErr != nil {
				return "", "", applyErr
			}
			return string(rewritten), rewrittenHash, nil
		},
	})
	if syncErr != nil {
		return nil, commonSettingsSaveError(syncErr, worksetID)
	}
	return s.GetOperation(ctx, worksetID, operationType)
}

// commonSettingsSaveError maps a refused global save onto the operation-scoped
// codes the draft endpoint already speaks. The record the user is editing
// keeps its own busy code, because that is the wait the user is looking at;
// every other refusal belongs to the save as a whole.
func commonSettingsSaveError(err error, worksetID string) error {
	if refusal, ok := errors.AsType[*CommonSettingsRefusalError](err); ok {
		if refusal.WorksetID == worksetID {
			if refusal.Reason == RefusalExecuting {
				return NewError(
					ErrKindConflict,
					"EXECUTION_IN_PROGRESS",
					"cancel or wait for the active execution before editing the draft",
					nil,
				)
			}
			return NewError(
				ErrKindConflict,
				"GENERATION_IN_PROGRESS",
				"cancel or wait for the active generation before editing the draft",
				nil,
			)
		}
		if refusal.Reason == RefusalUnreadableDraft {
			return NewError(
				ErrKindConflict,
				"COMMON_SETTINGS_UNREADABLE",
				"record "+refusal.WorksetID+" ("+refusal.Title+") holds a draft that cannot be read; "+
					"the global settings were not saved",
				nil,
			)
		}
		return NewError(
			ErrKindConflict,
			"COMMON_SETTINGS_BUSY",
			"record "+refusal.WorksetID+" ("+refusal.Title+") has a queued or running "+
				refusal.Reason+"; the global settings were not saved",
			nil,
		)
	}
	if errors.Is(err, ErrVersionConflict) {
		return NewError(ErrKindConflict, "VERSION_CONFLICT", "operation version conflict", nil)
	}
	if errors.Is(err, ErrOperationNotFound) {
		return NewError(ErrKindNotFound, "OPERATION_NOT_FOUND", "operation not found", nil)
	}
	return NewError(ErrKindInternal, "INTERNAL", "failed to save draft", err)
}
