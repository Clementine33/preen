package works

import (
	"encoding/json"
	"time"
)

// Optional marks a field a patch may leave alone. Set distinguishes "absent"
// from "present with a zero value", which is the whole difficulty of a partial
// update.
type Optional[T any] struct {
	Set   bool
	Value T
}

// NullableInt is an integer a request may omit, set, or explicitly clear.
type NullableInt struct {
	Set   bool
	Value *int
}

// UnmarshalJSON reads an integer, or an explicit null meaning "clear it". An
// absent field never reaches this method, so Set then means "the caller said
// something about this field".
func (n *NullableInt) UnmarshalJSON(data []byte) error {
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

// UserDataPatch is one partial user-data update. Only the fields the caller
// mentioned are applied.
type UserDataPatch struct {
	Rating   NullableInt
	Favorite Optional[bool]
	Note     Optional[string]
}

// UserData is the single user's data about one work. No row is stored until
// something is set, so the zero value is what a work without user data reads as.
type UserData struct {
	Rating   *int
	Favorite bool
	Note     string
}

// UserDataRequest is the PATCH body.
type UserDataRequest struct {
	Rating   NullableInt `json:"rating"`
	Favorite *bool       `json:"favorite"`
	Note     *string     `json:"note"`
}

// Patch converts the request into the patch the store applies, checking the
// rating range. Preferring a range check here rather than only in the table is
// what makes a bad value a clear 400 instead of a constraint failure.
func (r UserDataRequest) Patch() (UserDataPatch, *Error) {
	patch := UserDataPatch{}
	if r.Rating.Set {
		if r.Rating.Value != nil && (*r.Rating.Value < 1 || *r.Rating.Value > 10) {
			return patch, Invalid(CodeInvalidArgument, "rating must be an integer between 1 and 10")
		}
		patch.Rating = r.Rating
	}
	if r.Favorite != nil {
		patch.Favorite = Optional[bool]{Set: true, Value: *r.Favorite}
	}
	if r.Note != nil {
		patch.Note = Optional[string]{Set: true, Value: *r.Note}
	}
	return patch, nil
}

// UserTagsRequest is the PUT /works/{rj}/user-tags body: the complete set that
// replaces whatever relations the work had.
type UserTagsRequest struct {
	TagIDs []string `json:"tag_ids"`
}

// Page is one page of the ordinary listing plus how many works the whole
// filter lists, which is what a numbered pager needs.
type Page struct {
	Works []WorkSummary
	Total int
}

// Store is the catalog storage this package needs, declared here by its
// consumer. Every method is one use case on the business side; the SQL, the
// null handling and the transaction boundary stay with the adapter.
type Store interface {
	// ListWorks returns one page of the ordinary listing.
	ListWorks(filter Filter) (Page, error)
	// ListFacets returns the entities of one kind that the works the filter
	// lists carry, ordered by name. Order, page size and page number play no part.
	ListFacets(kind FacetKind, filter Filter) ([]Facet, error)
	// GetWork loads one work in full. The bool reports whether it exists.
	GetWork(id WorkCode) (*WorkDetail, bool, error)
	// ImportMetadata applies a validated submission in one transaction,
	// recomputing every derived relation the change affects.
	ImportMetadata(in *ValidatedImport, now time.Time) (ImportOutcome, error)
	// PatchUserData applies a partial update and returns the resulting state.
	PatchUserData(id WorkCode, patch UserDataPatch, now time.Time) (UserData, error)
	// ReplaceUserTags replaces the work's user-tag relations with exactly the
	// given set. Every id must exist.
	ReplaceUserTags(id WorkCode, tagIDs []string) ([]UserTag, error)

	// ListUserTags returns every user tag, ordered by name.
	ListUserTags() ([]UserTag, error)
	// CreateUserTag stores a new tag. The bool reports whether it was created;
	// an existing tag with the same name is returned instead.
	CreateUserTag(name string) (UserTag, bool, error)
	// RenameUserTag renames a tag, refusing a name another tag holds.
	RenameUserTag(id, name string) (UserTag, error)
	// DeleteUserTag removes a tag and its relations.
	DeleteUserTag(id string) error

	// TrackFormats reads the stored Format Preference; nil when never saved.
	TrackFormats() ([]string, error)
	// SaveTrackFormats replaces the stored Format Preference.
	SaveTrackFormats(formats []string, now time.Time) error

	// UpstreamProxy reads the stored Upstream Proxy URL; "" when never saved
	// or direct.
	UpstreamProxy() (string, error)
	// SaveUpstreamProxy replaces the stored Upstream Proxy URL.
	SaveUpstreamProxy(url string, now time.Time) error

	// DeleteWork permanently removes a work. It returns a WORK_IN_USE refusal
	// when a copy or another work's evidence still points at it.
	DeleteWork(id WorkCode) *Error

	// WorkCopyRoots reads the copies of one work joined to their library
	// roots, the join the content manifest builds each copy's tree from. The
	// bool reports whether the work exists; a work with no copies is found
	// with an empty list.
	WorkCopyRoots(id WorkCode) ([]CopyRoot, bool, error)
	// WorkAt reads the work a library directory is bound to by its copy. The
	// bool reports whether the directory is bound at all.
	WorkAt(libraryID, relPath string) (WorkCode, bool, error)
}
