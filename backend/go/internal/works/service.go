package works

import "time"

// Service is the catalog entry the HTTP layer calls. The rules live in the
// pure functions of this package; the service composes them with storage.
type Service struct {
	store Store
	// entries, resolveMember and resolveMemberFile are the content
	// manifest's disk-and-inventory side (content.go): the stored entries,
	// whether a member root is currently on disk, and the containment-checked
	// resolution of one manifest file.
	entries           ContentEntries
	resolveMember     ResolveMember
	resolveMemberFile ResolveMemberFile
	// thumbEngine is the thumbnail engine's version, part of every image's
	// thumbnail address (ADR 0013).
	thumbEngine string
	// now is the clock, injectable so a test can pin the timestamps a write
	// records.
	now func() time.Time
}

// NewService creates the catalog entry. entries serves the content manifest;
// resolveMember decides a copy's online flag; resolveMemberFile resolves one
// manifest path for byte serving (ADR 0013); thumbEngine versions the
// thumbnail addresses (ADR 0013).
func NewService(
	store Store,
	entries ContentEntries,
	resolveMember ResolveMember,
	resolveMemberFile ResolveMemberFile,
	thumbEngine string,
) *Service {
	return &Service{
		store:             store,
		entries:           entries,
		resolveMember:     resolveMember,
		resolveMemberFile: resolveMemberFile,
		thumbEngine:       thumbEngine,
		now:               time.Now,
	}
}

// List returns one page of the ordinary listing and the listing's total. It
// validates the filter and hands the storage a filter it can turn into SQL
// directly.
func (s *Service) List(filter Filter) (Page, *Error) {
	if err := filter.Validate(); err != nil {
		return Page{}, err
	}
	page, err := s.store.ListWorks(filter)
	if err != nil {
		return Page{}, asInternal(err)
	}
	return page, nil
}

// Detail returns one collected work in full.
func (s *Service) Detail(id WorkCode) (*WorkDetail, *Error) {
	detail, found, err := s.store.GetWork(id)
	if err != nil {
		return nil, asInternal(err)
	}
	if !found {
		return nil, NotFound(CodeWorkNotFound, "no such work")
	}
	return detail, nil
}

// Import applies one metadata submission. The contract is checked here, so an
// illegal body never reaches the transaction; the group rules and the relation
// rules are applied by the storage, which is the only place with the rest of
// the catalog in view.
func (s *Service) Import(target WorkCode, req ImportRequest) (ImportOutcome, *Error) {
	validated, invalid := ValidateImport(target, req)
	if invalid != nil {
		return ImportOutcome{}, invalid
	}
	outcome, err := s.store.ImportMetadata(validated, s.now())
	if err != nil {
		return ImportOutcome{}, asInternal(err)
	}
	return outcome, nil
}

// PatchUserData applies a partial update. A missing work is a 404 rather than a
// silent no-op, because a caller that names a work expects it to exist.
func (s *Service) PatchUserData(id WorkCode, req UserDataRequest) (UserData, *Error) {
	patch, invalid := req.Patch()
	if invalid != nil {
		return UserData{}, invalid
	}
	data, err := s.store.PatchUserData(id, patch, s.now())
	if err != nil {
		return UserData{}, asInternal(err)
	}
	return data, nil
}

// PutListening stores one Listening Position snapshot. Like PatchUserData, an
// unknown work is a 404 (decided by the storage).
func (s *Service) PutListening(id WorkCode, req ListeningRequest) (Listening, *Error) {
	if invalid := req.Validate(); invalid != nil {
		return Listening{}, invalid
	}
	row, err := s.store.PutListening(id, req, s.now())
	if err != nil {
		return Listening{}, asInternal(err)
	}
	return row, nil
}

// ClearListening forgets every Listening Position and Finished mark of the work.
func (s *Service) ClearListening(id WorkCode) *Error {
	if err := s.store.ClearListening(id); err != nil {
		return asInternal(err)
	}
	return nil
}

// ReplaceUserTags points the work at exactly the given tags.
func (s *Service) ReplaceUserTags(id WorkCode, req UserTagsRequest) ([]UserTag, *Error) {
	tags, err := s.store.ReplaceUserTags(id, req.TagIDs)
	if err != nil {
		return nil, asInternal(err)
	}
	return tags, nil
}

// UserTags lists the user's tags.
func (s *Service) UserTags() ([]UserTag, *Error) {
	tags, err := s.store.ListUserTags()
	if err != nil {
		return nil, asInternal(err)
	}
	return tags, nil
}

// CreateUserTag stores a new tag. A name that already exists returns that tag
// unchanged: creating a tag is idempotent by name, so a caller does not have to
// look one up before using it.
func (s *Service) CreateUserTag(raw string) (UserTag, *Error) {
	name, ok := NormalizeUserTagName(raw)
	if !ok {
		return UserTag{}, Invalid(CodeInvalidTagName, "a tag needs a name")
	}
	tag, _, err := s.store.CreateUserTag(name)
	if err != nil {
		return UserTag{}, asInternal(err)
	}
	return tag, nil
}

// RenameUserTag renames one tag.
func (s *Service) RenameUserTag(id, raw string) (UserTag, *Error) {
	name, ok := NormalizeUserTagName(raw)
	if !ok {
		return UserTag{}, Invalid(CodeInvalidTagName, "a tag needs a name")
	}
	tag, err := s.store.RenameUserTag(id, name)
	if err != nil {
		return UserTag{}, asInternal(err)
	}
	return tag, nil
}

// DeleteUserTag removes one tag and its relations.
func (s *Service) DeleteUserTag(id string) *Error {
	if err := s.store.DeleteUserTag(id); err != nil {
		return asInternal(err)
	}
	return nil
}

// Delete removes a work only when no copy or personal data protects it.
func (s *Service) Delete(id WorkCode) *Error {
	return s.store.DeleteWork(id)
}

// asInternal wraps an unexpected storage failure. A refusal the storage already
// classified passes through untouched.
func asInternal(err error) *Error {
	if classified, ok := AsError(err); ok {
		return classified
	}
	return &Error{
		Kind:    KindInternal,
		Code:    "INTERNAL",
		Message: "the catalog could not complete the request",
		Cause:   err,
	}
}

// WorkAt answers the work a library directory is bound to, if any. Binding is
// maintained by scanning, so an unbound directory is an ordinary answer.
func (s *Service) WorkAt(libraryID, relPath string) (WorkCode, bool, error) {
	return s.store.WorkAt(libraryID, relPath)
}
