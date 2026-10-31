package workset

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/librarypath"
)

// MaxMembers caps the ordered album folders in one workset.
const MaxMembers = 10000

// idemRetentionWindow is the guaranteed replay window for workset creation and
// generation idempotency keys, and the terminal-generation retention horizon.
const idemRetentionWindow = 30 * 24 * time.Hour

// newToken produces a sortable, collision-resistant identifier: a nanosecond
// timestamp plus a random suffix. The timestamp alone collides when two
// aggregates are created within the same nanosecond (e.g. test loops), which
// surfaced as a spurious idempotency-key conflict on the primary key.
func newToken() string {
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		// Fall back to the timestamp-only form rather than failing creation.
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(rnd[:]))
}

// CreateCurrentWorkset creates the library's shared folder batch, replacing
// the record the caller saw as current. Every registered task uses its members.
//
// The requested scope is validated against the scanned inventory — the same
// listing the caller selected from — and every directory that does not become
// a member is reported with its reason instead of being silently dropped. A
// request whose whole selection is unusable creates nothing; a request that
// replaces a busy record (queued or running planning or execution) is refused
// and leaves that record untouched.
func (s *serviceImpl) CreateCurrentWorkset(
	ctx context.Context,
	req CreateCurrentRequest,
) (*CreateCurrentResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := s.requireTask(req.OperationType); err != nil {
		return nil, err
	}
	if err := validateSelection(req.Selection); err != nil {
		return nil, err
	}
	if idemErr := validateIdemKey(req.IdempotencyKey); idemErr != nil {
		return nil, idemErr
	}
	lib, err := s.repo.GetLibrary(req.LibraryID)
	if err != nil {
		if errors.Is(err, library.ErrLibraryNotFound) {
			return nil, NewError(ErrKindNotFound, "LIBRARY_NOT_FOUND", "library not found", nil)
		}
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load library", err)
	}

	members, skipped, err := s.resolveSelectedMembers(lib, req.Selection)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, NewError(
			ErrKindInvalidArgument,
			"NO_MEMBERS",
			"none of the selected folders can join this workset",
			nil,
		).WithDetails(skipPaths(skipped))
	}
	if len(members) > MaxMembers {
		return nil, NewError(
			ErrKindInvalidArgument,
			"INVALID_FOLDER_COUNT",
			fmt.Sprintf("a record holds at most %d member folders", MaxMembers),
			nil,
		)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = lib.Name
	}
	if err := validateTitle(title); err != nil {
		return nil, err
	}
	requestHash := createRequestHash(req)
	if result, replayed, replayErr := s.replayCreate(
		ctx,
		lib,
		req,
		requestHash,
		skipped,
	); replayErr != nil ||
		replayed {
		return result, replayErr
	}
	return s.persistCurrentWorkset(ctx, lib, req, requestHash, title, members, skipped)
}

// replayCreate answers a repeated creation request with the record its
// idempotency key already owns, provided that record is still the current one
// and the request is the same request. A key that now belongs to replaced
// content, or a key reused for a different request, is a conflict: neither
// resurrects the record it replaced nor replaces the current record again.
func (s *serviceImpl) replayCreate(
	ctx context.Context,
	lib *library.Library,
	req CreateCurrentRequest,
	requestHash string,
	skipped []SkippedFolder,
) (*CreateCurrentResult, bool, error) {
	if req.IdempotencyKey == "" {
		return nil, false, nil
	}
	existing, err := s.repo.GetWorksetByCreationIdemKey(req.IdempotencyKey)
	if err != nil {
		return nil, false, NewError(ErrKindInternal, "INTERNAL", "failed to check idempotency key", err)
	}
	if existing == nil {
		return nil, false, nil
	}
	if time.Since(existing.CreatedAt) >= idemRetentionWindow {
		// Expired key: release ownership so a retry creates a fresh record.
		if expireErr := s.repo.ClearExpiredWorksetIdemKey(
			existing.ID,
			time.Now().Add(-idemRetentionWindow),
		); expireErr != nil {
			return nil, false, NewError(ErrKindInternal, "INTERNAL", "failed to expire idempotency key", expireErr)
		}
		return nil, false, nil
	}
	if existing.CreationRequestHash != requestHash {
		return nil, false, NewError(
			ErrKindConflict,
			"IDEMPOTENCY_KEY_REUSED",
			"this idempotency key was used for a different request",
			nil,
		)
	}
	current, err := s.repo.GetCurrentWorkset(lib.ID)
	if err != nil {
		return nil, false, NewError(ErrKindInternal, "INTERNAL", "failed to load the current record", err)
	}
	if current == nil || current.ID != existing.ID {
		return nil, false, NewError(
			ErrKindConflict,
			"RECORD_REPLACED",
			"this creation request has already been replaced by a newer record",
			nil,
		)
	}
	view, err := s.GetWorkset(ctx, existing.ID)
	if err != nil {
		return nil, false, err
	}
	return &CreateCurrentResult{
		Workset:  view,
		Created:  false,
		Recorded: len(view.Members),
		Skipped:  skipped,
	}, true, nil
}

// persistCurrentWorkset writes the new record and retires the one it replaces
// in one transaction.
func (s *serviceImpl) persistCurrentWorkset(
	ctx context.Context,
	lib *library.Library,
	req CreateCurrentRequest,
	requestHash, title string,
	members []WorksetMember,
	skipped []SkippedFolder,
) (*CreateCurrentResult, error) {
	now := time.Now()
	ws := &Workset{
		ID:                  "ws-" + newToken(),
		Title:               title,
		LibraryID:           lib.ID,
		RootPath:            lib.RootPath,
		RootPathKey:         librarypath.RootPathKey(lib.RootPath),
		Version:             1,
		CreationIdemKey:     req.IdempotencyKey,
		CreationRequestHash: requestHash,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	rows := make([]WorksetMember, 0, len(members))
	for i, m := range members {
		m.WorksetID = ws.ID
		m.MemberID = "m-" + newToken()
		m.MemberIndex = i
		rows = append(rows, m)
	}
	// Tasks share members but retain independent drafts, versions and plans.
	ops := make([]Operation, 0, len(s.tasks))
	drafts := make([]OperationDraft, 0, len(s.tasks))
	for _, task := range s.tasks {
		seedCommon, err := s.seedCommonSettings(task, lib.ID, task.Kind(), req.ExpectedCurrentID)
		if err != nil {
			return nil, err
		}
		raw, hash, schemaVersion := task.SeedDraft(seedCommon)
		ops = append(ops, Operation{
			WorksetID: ws.ID, OperationType: task.Kind(), Version: 1,
			CreatedAt: now, UpdatedAt: now,
		})
		drafts = append(drafts, OperationDraft{
			WorksetID: ws.ID, OperationType: task.Kind(), SchemaVersion: schemaVersion,
			DraftJSON: string(raw), DraftHash: hash, UpdatedAt: now,
		})
	}
	if replaceErr := s.repo.ReplaceCurrentWorkset(ws, rows, ops, drafts, req.ExpectedCurrentID); replaceErr != nil {
		switch {
		case errors.Is(replaceErr, ErrWorksetIdemConflict):
			if result, replayed, _ := s.replayCreate(ctx, lib, req, requestHash, skipped); replayed {
				return result, nil
			}
			return nil, NewError(ErrKindConflict, "IDEMPOTENCY_KEY_REUSED", "idempotency key conflict", replaceErr)
		case errors.Is(replaceErr, ErrCurrentRecordChanged):
			return nil, NewError(
				ErrKindConflict,
				"RECORD_REPLACED",
				"the current record changed while this request was in flight",
				replaceErr,
			)
		case errors.Is(replaceErr, ErrRecordBusy):
			return nil, NewError(
				ErrKindConflict,
				"RECORD_BUSY",
				"the record being replaced has a queued or running session",
				replaceErr,
			)
		}
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to create the current record", replaceErr)
	}
	view, err := s.GetWorkset(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	return &CreateCurrentResult{Workset: view, Created: true, Recorded: len(rows), Skipped: skipped}, nil
}

// validateSelection checks the shape of the selection and the explicit id
// count; the resolved member count is checked after resolution.
func validateSelection(sel Selection) error {
	if sel.All && len(sel.DirIDs) > 0 || !sel.All && len(sel.ExceptDirIDs) > 0 {
		return NewError(
			ErrKindInvalidArgument,
			"INVALID_SELECTION",
			"a selection is either all with except_dir_ids, or dir_ids",
			nil,
		)
	}
	if !sel.All && (len(sel.DirIDs) == 0 || len(sel.DirIDs) > MaxMembers) {
		return NewError(
			ErrKindInvalidArgument,
			"INVALID_FOLDER_COUNT",
			fmt.Sprintf("a record requires between 1 and %d member folders", MaxMembers),
			nil,
		)
	}
	return nil
}

// resolveSelectedMembers turns the selection into ordered members in O(N): the
// directory id of every scanned work root is derived once into a lookup, then
// each selected id is a map read. Every id that cannot join is reported with
// its reason rather than dropped in silence; `all` is whatever the inventory
// holds when the request is read. Members follow inventory path order, and
// membership is independent of task kind.
func (s *serviceImpl) resolveSelectedMembers(
	lib *library.Library,
	sel Selection,
) ([]WorksetMember, []SkippedFolder, error) {
	roots, err := s.repo.ListLibraryWorkRoots(lib.RootPath)
	if err != nil {
		return nil, nil, NewError(ErrKindInternal, "INTERNAL", "failed to read work roots", err)
	}
	ids := make([]string, len(roots))
	index := make(map[string]int, len(roots))
	ambiguous := make(map[string]bool)
	for i, rel := range roots {
		id := library.DirID(lib.ID, lib.RootPath, rel)
		ids[i] = id
		if _, dup := index[id]; dup {
			ambiguous[id] = true
			continue
		}
		index[id] = i
	}

	skipped := make([]SkippedFolder, 0)
	picked := make([]int, 0, len(roots))
	if sel.All {
		except := make(map[string]bool, len(sel.ExceptDirIDs))
		for _, id := range sel.ExceptDirIDs {
			except[id] = true
		}
		reported := make(map[string]bool)
		for i, id := range ids {
			switch {
			case except[id]:
			case ambiguous[id]:
				if !reported[id] {
					reported[id] = true
					skipped = append(skipped, SkippedFolder{DirID: id, Reason: SkipAmbiguous})
				}
			default:
				picked = append(picked, i)
			}
		}
	} else {
		seen := make(map[string]bool, len(sel.DirIDs))
		for _, id := range sel.DirIDs {
			i, known := index[id]
			switch {
			case seen[id]:
				skipped = append(skipped, SkippedFolder{DirID: id, Reason: SkipDuplicate})
			case ambiguous[id]:
				skipped = append(skipped, SkippedFolder{DirID: id, Reason: SkipAmbiguous})
			case !known:
				skipped = append(skipped, SkippedFolder{DirID: id, Reason: SkipUnknownDir})
			default:
				picked = append(picked, i)
			}
			seen[id] = true
		}
		sort.Ints(picked)
	}
	out := make([]WorksetMember, 0, len(picked))
	for _, i := range picked {
		rel := roots[i]
		out = append(out, WorksetMember{
			RelPath:    rel,
			FolderPath: librarypath.JoinRel(lib.RootPath, rel),
			FolderName: path.Base(rel),
		})
	}
	return out, skipped, nil
}

// skipPaths lists the skipped ids as error details, so a refused creation
// still names what it could not use.
func skipPaths(skipped []SkippedFolder) []string {
	out := make([]string, 0, len(skipped))
	for _, s := range skipped {
		out = append(out, s.DirID+": "+s.Reason)
	}
	return out
}

// createRequestHash is the canonical identity of one creation request. The
// ids hash as a set: the order the caller selected in is not part of the
// request, so a retry that reorders the same selection still replays instead
// of conflicting.
func createRequestHash(req CreateCurrentRequest) string {
	sel := req.Selection
	listed := sel.DirIDs
	if sel.All {
		listed = sel.ExceptDirIDs
	}
	ids := slices.Clone(listed)
	slices.Sort(ids)
	raw, err := json.Marshal(struct {
		LibraryID         string   `json:"library_id"`
		All               bool     `json:"all"`
		DirIDs            []string `json:"dir_ids"`
		ExpectedCurrentID string   `json:"expected_current_id"`
	}{req.LibraryID, sel.All, slices.Compact(ids), req.ExpectedCurrentID})
	if err != nil {
		return ""
	}
	return CanonicalJSONHash(raw)
}

// GetCurrentWorkset returns the library's shared batch, or nil when absent.
// operationType validates the task entry point; it does not select a batch.
func (s *serviceImpl) GetCurrentWorkset(
	ctx context.Context,
	libraryID, operationType string,
) (*WorksetView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := s.requireTask(operationType); err != nil {
		return nil, err
	}
	w, err := s.repo.GetCurrentWorkset(libraryID)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load the current record", err)
	}
	if w == nil {
		return nil, nil
	}
	return s.view(w)
}

func validateTitle(title string) error {
	if title == "" {
		return NewError(ErrKindInvalidArgument, "INVALID_TITLE", "title is required", nil)
	}
	n := utf8.RuneCountInString(title)
	if n < 1 || n > 120 {
		return NewError(ErrKindInvalidArgument, "INVALID_TITLE", "title must be 1-120 characters", nil)
	}
	return nil
}

func validateIdemKey(key string) error {
	if key != "" && (len(key) > 255 || strings.ContainsAny(key, " \t\r\n")) {
		return NewError(
			ErrKindInvalidArgument,
			"INVALID_IDEMPOTENCY_KEY",
			"idempotency key is too long or malformed",
			nil,
		)
	}
	return nil
}

// RenameWorkset renames the workset (always allowed, even during
// generation). It is guarded by the workset metadata version, never by an
// operation version.
func (s *serviceImpl) RenameWorkset(ctx context.Context, id string, req RenameRequest) (*WorksetView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(req.Title)
	if err := validateTitle(title); err != nil {
		return nil, err
	}
	w, err := s.loadWorkset(id)
	if err != nil {
		return nil, err
	}
	if w.LibraryID == "" {
		return nil, NewError(ErrKindConflict, "ORPHANED_WORKSET", "orphaned worksets are read-only", nil)
	}
	if err := s.repo.RenameWorkset(id, title, req.IfMatchVersion, time.Now()); err != nil {
		if errors.Is(err, ErrVersionConflict) {
			return nil, NewError(ErrKindConflict, "VERSION_CONFLICT", "workset version conflict", nil)
		}
		if errors.Is(err, ErrWorksetNotFound) {
			return nil, NewError(ErrKindNotFound, "WORKSET_NOT_FOUND", "workset not found", nil)
		}
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to rename workset", err)
	}
	return s.GetWorkset(ctx, id)
}
