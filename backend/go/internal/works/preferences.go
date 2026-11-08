package works

import (
	"errors"
	"strings"

	"github.com/Clementine33/preen/backend/internal/library"
)

// Preferences are the single user's deployment-wide display choices. They sync
// through the server rather than the browser, so every device lists and plays
// the same format and opens the same Library first.
type Preferences struct {
	// TrackFormats is the ordered Format Preference: the first listed format a
	// work ships is its effective format. Empty means no preference.
	TrackFormats []string `json:"track_formats"`
	// DefaultLibraryID is the Default Library; absent when none is set. It may
	// name a Library deleted since, which readers treat as none.
	DefaultLibraryID *string `json:"default_library_id,omitempty"`
}

// PreferencesUpdate is one merge into the stored preferences: a field the body
// leaves out stays as it is, so the settings pages never overwrite each other.
type PreferencesUpdate struct {
	TrackFormats     *[]string      `json:"track_formats"`
	DefaultLibraryID NullableString `json:"default_library_id"`
}

// maxTrackFormats bounds the list so a malformed client cannot store a blob.
const maxTrackFormats = 32

// normalizeFormats trims and uppercases each format (the labels containerName
// produces), drops later duplicates and refuses empty entries.
func normalizeFormats(raw []string) ([]string, *Error) {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		format := strings.ToUpper(strings.TrimSpace(item))
		if format == "" {
			return nil, Invalid(CodeInvalidPreferences, "a format must not be empty")
		}
		if !seen[format] {
			seen[format] = true
			out = append(out, format)
		}
	}
	if len(out) > maxTrackFormats {
		return nil, Invalid(CodeInvalidPreferences, "too many formats")
	}
	return out, nil
}

// Preferences reads the stored preferences; never saved reads as no preference.
func (s *Service) Preferences() (Preferences, *Error) {
	prefs, err := s.store.Preferences()
	if err != nil {
		return Preferences{}, asInternal(err)
	}
	if prefs.TrackFormats == nil {
		prefs.TrackFormats = []string{}
	}
	return prefs, nil
}

// SavePreferences merges the update into the stored preferences (last write
// wins per field) and returns the normalized result. Nothing is stored on
// refusal.
func (s *Service) SavePreferences(update PreferencesUpdate) (Preferences, *Error) {
	if update.TrackFormats == nil && !update.DefaultLibraryID.Set {
		return Preferences{}, Invalid(CodeInvalidPreferences, "no preference field given")
	}
	if update.TrackFormats != nil {
		formats, refusal := normalizeFormats(*update.TrackFormats)
		if refusal != nil {
			return Preferences{}, refusal
		}
		update.TrackFormats = &formats
	}
	if id := update.DefaultLibraryID.Value; id != nil {
		if *id == "" {
			return Preferences{}, Invalid(CodeInvalidPreferences, "default_library_id must not be empty")
		}
		if _, err := s.store.GetLibrary(*id); errors.Is(err, library.ErrLibraryNotFound) {
			return Preferences{}, NotFound(CodeLibraryNotFound, "library not found")
		} else if err != nil {
			return Preferences{}, asInternal(err)
		}
	}
	if err := s.store.SavePreferences(update, s.now()); err != nil {
		return Preferences{}, asInternal(err)
	}
	return s.Preferences()
}
