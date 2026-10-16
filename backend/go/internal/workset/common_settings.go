package workset

import (
	"encoding/json"
	"errors"
	"time"
)

// Why one record refuses a deployment-wide settings save. The save is refused as
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

// CommonSettingsSync is one deployment-wide common-settings save, through
// PUT /api/v1/tasks/{type}/settings. The store applies the whole payload as one
// transaction (ADR 0008): the stored payload and the common fields of every
// editable operation of the kind, each with its draft hash and operation
// version.
type CommonSettingsSync struct {
	OperationType string
	// ExpectedHash is the canonical-JSON hash the caller read the settings at
	// (the ETag it echoed). Without a stored payload the current value is
	// SeedSettings; a different hash refuses the save with ErrVersionConflict.
	ExpectedHash string
	SeedSettings string
	// Settings is the new deployment-wide common-settings payload.
	Settings string
	// Rewrite returns one operation's new canonical document and hash with
	// Settings applied to its stored one, preserving its member records. A
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

// commonSettingsSaveError maps a refused settings save onto its stable codes.
// The save is refused as a whole, whichever record caused it.
func commonSettingsSaveError(err error) error {
	if refusal, ok := errors.AsType[*CommonSettingsRefusalError](err); ok {
		if refusal.Reason == RefusalUnreadableDraft {
			return NewError(
				ErrKindConflict,
				"COMMON_SETTINGS_UNREADABLE",
				"record "+refusal.WorksetID+" ("+refusal.Title+") holds a draft that cannot be read; "+
					"the settings were not saved",
				nil,
			)
		}
		return NewError(
			ErrKindConflict,
			"COMMON_SETTINGS_BUSY",
			"record "+refusal.WorksetID+" ("+refusal.Title+") has a queued or running "+
				refusal.Reason+"; the settings were not saved",
			nil,
		)
	}
	if errors.Is(err, ErrVersionConflict) {
		return NewError(ErrKindConflict, "VERSION_CONFLICT", "the settings were changed elsewhere", nil)
	}
	return NewError(ErrKindInternal, "INTERNAL", "failed to save the settings", err)
}
