package workset

import (
	"context"
	"encoding/json"
	"time"
)

// TaskSettings is the deployment-wide common settings of one Task kind as the
// settings endpoint serves them. ETag is the canonical-JSON hash of the stored
// payload (of the task's seed value while none was saved): the If-Match the
// next save must echo (ADR 0008).
type TaskSettings struct {
	OperationType string
	Settings      json.RawMessage
	ETag          string
}

// GetTaskSettings returns the stored common settings of one Task kind, or the
// Task's seed value when none were saved. It needs no record.
func (s *serviceImpl) GetTaskSettings(ctx context.Context, operationType string) (*TaskSettings, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	task, err := s.requireTask(operationType)
	if err != nil {
		return nil, err
	}
	raw, err := s.currentSettings(task)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeSettings(task, raw)
	if err != nil {
		return nil, NewError(ErrKindConflict, "COMMON_SETTINGS_UNREADABLE", "the stored settings cannot be read", err)
	}
	return &TaskSettings{OperationType: operationType, Settings: normalized, ETag: CanonicalJSONHash(raw)}, nil
}

// SaveTaskSettings is the only write path of a Task kind's common settings: it
// stores them and rewrites the common section of every editable record of the
// kind in one transaction, or changes nothing (ADR 0008). ifMatch is the ETag
// the caller read. Plans already generated stay frozen; the rewritten drafts
// are what send them to needs_planning.
func (s *serviceImpl) SaveTaskSettings(
	ctx context.Context,
	operationType string,
	payload json.RawMessage,
	ifMatch string,
) (*TaskSettings, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	task, err := s.requireTask(operationType)
	if err != nil {
		return nil, err
	}
	seed, _, _ := task.SeedDraft(nil)
	candidate, _, applyErr := task.ApplyCommonSettings(seed, payload)
	if applyErr != nil {
		if werr, ok := AsError(applyErr); ok && werr.Kind != ErrKindInternal {
			return nil, applyErr
		}
		return nil, NewError(ErrKindInvalidArgument, "INVALID_SETTINGS", "the settings are not valid", applyErr)
	}
	if validateErr := task.ValidateDraft(candidate, nil); validateErr != nil {
		return nil, validateErr
	}
	normalized, err := task.CommonSettings(candidate)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to read the settings back", err)
	}
	seedSettings, err := task.CommonSettings(seed)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to read the seed settings", err)
	}
	syncErr := s.repo.SyncCommonSettings(CommonSettingsSync{
		OperationType: operationType,
		ExpectedHash:  ifMatch,
		SeedSettings:  string(seedSettings),
		Settings:      string(normalized),
		Now:           time.Now(),
		Rewrite: func(document string) (string, string, error) {
			rewritten, hash, rewriteErr := task.ApplyCommonSettings([]byte(document), normalized)
			if rewriteErr != nil {
				return "", "", rewriteErr
			}
			return string(rewritten), hash, nil
		},
	})
	if syncErr != nil {
		return nil, commonSettingsSaveError(syncErr)
	}
	return &TaskSettings{
		OperationType: operationType,
		Settings:      normalized,
		ETag:          CanonicalJSONHash(normalized),
	}, nil
}

// currentSettings is the stored payload, or the Task's seed value.
func (s *serviceImpl) currentSettings(task Task) (json.RawMessage, error) {
	stored, err := s.repo.GetCommonSettings(task.Kind())
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load the common settings", err)
	}
	if stored != nil {
		return json.RawMessage(stored.Settings), nil
	}
	seed, _, _ := task.SeedDraft(nil)
	raw, err := task.CommonSettings(seed)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to read the seed settings", err)
	}
	return raw, nil
}

// normalizeSettings reads a common-settings payload the way the task writes it
// today, so an older stored spelling compares equal to the same settings.
func normalizeSettings(task Task, payload json.RawMessage) (json.RawMessage, error) {
	seed, _, _ := task.SeedDraft(nil)
	doc, _, err := task.ApplyCommonSettings(seed, payload)
	if err != nil {
		return nil, err
	}
	return task.CommonSettings(doc)
}

// rejectDivergentCommon refuses a draft save whose common section differs from
// the deployment's: settings are written through the settings endpoint only.
// While no settings were ever saved, the record's own stored draft is the
// reference, so member-only edits keep working.
func (s *serviceImpl) rejectDivergentCommon(task Task, worksetID string, canonical []byte) error {
	submitted, err := task.CommonSettings(canonical)
	if err != nil {
		return err
	}
	stored, err := s.repo.GetCommonSettings(task.Kind())
	if err != nil {
		return NewError(ErrKindInternal, "INTERNAL", "failed to load the common settings", err)
	}
	var expected json.RawMessage
	if stored != nil {
		expected, err = normalizeSettings(task, json.RawMessage(stored.Settings))
	} else {
		draft, draftErr := s.repo.GetOperationDraft(worksetID, task.Kind())
		if draftErr != nil {
			return NewError(ErrKindInternal, "INTERNAL", "failed to load draft", draftErr)
		}
		if draft == nil {
			return NewError(ErrKindConflict, "COMMON_SETTINGS_UNREADABLE", "record "+worksetID+" has no draft", nil)
		}
		expected, err = task.CommonSettings([]byte(draft.DraftJSON))
	}
	if err != nil {
		return NewError(ErrKindConflict, "COMMON_SETTINGS_UNREADABLE", "the stored settings cannot be read", err)
	}
	if CanonicalJSONHash(submitted) != CanonicalJSONHash(expected) {
		return NewError(
			ErrKindConflict,
			"COMMON_SETTINGS_MISMATCH",
			"the draft's common settings differ from the Task settings; save them through PUT /api/v1/tasks/{type}/settings",
			nil,
		)
	}
	return nil
}
