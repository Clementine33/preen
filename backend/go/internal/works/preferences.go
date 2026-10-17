package works

import (
	"strings"
)

// Preferences are the single user's deployment-wide display choices. They sync
// through the server rather than the browser, so every device lists and plays
// the same format.
type Preferences struct {
	// TrackFormats is the ordered Format Preference: the first listed format a
	// work ships is its effective format. Empty means no preference.
	TrackFormats []string `json:"track_formats"`
}

// maxTrackFormats bounds the list so a malformed client cannot store a blob.
const maxTrackFormats = 32

// Normalize trims and uppercases each format (the labels containerName
// produces), drops later duplicates and refuses empty entries.
func (p Preferences) Normalize() (Preferences, *Error) {
	if p.TrackFormats == nil {
		return p, Invalid(CodeInvalidPreferences, "track_formats is required")
	}
	out := make([]string, 0, len(p.TrackFormats))
	seen := map[string]bool{}
	for _, raw := range p.TrackFormats {
		format := strings.ToUpper(strings.TrimSpace(raw))
		if format == "" {
			return p, Invalid(CodeInvalidPreferences, "a format must not be empty")
		}
		if !seen[format] {
			seen[format] = true
			out = append(out, format)
		}
	}
	if len(out) > maxTrackFormats {
		return p, Invalid(CodeInvalidPreferences, "too many formats")
	}
	return Preferences{TrackFormats: out}, nil
}

// Preferences reads the stored preferences; never saved reads as no preference.
func (s *Service) Preferences() (Preferences, *Error) {
	formats, err := s.store.TrackFormats()
	if err != nil {
		return Preferences{}, asInternal(err)
	}
	if formats == nil {
		formats = []string{}
	}
	return Preferences{TrackFormats: formats}, nil
}

// SavePreferences replaces the stored preferences (last write wins) and returns
// the normalized result.
func (s *Service) SavePreferences(req Preferences) (Preferences, *Error) {
	normalized, refusal := req.Normalize()
	if refusal != nil {
		return Preferences{}, refusal
	}
	if err := s.store.SaveTrackFormats(normalized.TrackFormats, s.now()); err != nil {
		return Preferences{}, asInternal(err)
	}
	return normalized, nil
}
