package dirrename

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// Common holds the naming template and title filters shared by all members.
type Common struct {
	Template            string `json:"template"`
	StripBracketedTitle bool   `json:"strip_bracketed_title,omitempty"`
	StripTildeTitle     bool   `json:"strip_tilde_title,omitempty"`
	StripBookTitle      bool   `json:"strip_book_title,omitempty"`
	// VoiceActorSeparator joins the voice actors of `{voice_actors}`; empty means
	// the default "、". Other placeholders hold one value and need none.
	VoiceActorSeparator string `json:"voice_actor_separator,omitempty"`
}

const defaultActorSeparator = "、"

func (c Common) actorSeparator() string {
	if c.VoiceActorSeparator == "" {
		return defaultActorSeparator
	}
	return c.VoiceActorSeparator
}

// validateCommon checks the template and the separator, which ends up inside a
// directory name and so may not carry a character a name cannot hold.
func validateCommon(c Common) error {
	if err := validateTemplate(c.Template); err != nil {
		return err
	}
	if utf8.RuneCountInString(c.VoiceActorSeparator) > 16 ||
		strings.ContainsFunc(c.VoiceActorSeparator, func(r rune) bool {
			return r < 32 || strings.ContainsRune(`<>:"/\|?*`, r)
		}) {
		return invalid("voice actor separator is too long or has a character a name cannot hold")
	}
	return nil
}

// Member records an exclusion or a literal name for one folder. A named folder
// skips the template; the name still has to carry exactly one work code.
type Member struct {
	MemberID string `json:"member_id"`
	Excluded bool   `json:"excluded"`
	Name     string `json:"name,omitempty"`
}

// Draft is the versioned, sparse rename configuration.
type Draft struct {
	SchemaVersion int      `json:"schema_version"`
	Common        Common   `json:"common"`
	Members       []Member `json:"members"`
}

func (*Task) SeedDraft(common json.RawMessage) ([]byte, string, int) {
	d := Draft{SchemaVersion: 1, Common: Common{Template: "[{rj}] {title}"}, Members: []Member{}}
	if len(common) > 0 {
		var c Common
		if json.Unmarshal(common, &c) == nil && validateCommon(c) == nil {
			d.Common = c
		}
	}
	raw, _ := json.Marshal(d)
	return raw, workset.CanonicalJSONHash(raw), 1
}

func decodeDraft(raw []byte) (Draft, error) {
	var d Draft
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, invalid(err.Error())
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return d, invalid("unexpected trailing content")
	}
	if d.SchemaVersion != 1 {
		return d, workset.NewError(
			workset.ErrKindInvalidArgument,
			"UNSUPPORTED_SCHEMA_VERSION",
			"unsupported rename draft version",
			nil,
		)
	}
	return d, nil
}
func invalid(message string) error {
	return workset.NewError(workset.ErrKindInvalidArgument, "INVALID_DRAFT", message, nil)
}

func (*Task) CommonSettings(raw []byte) (json.RawMessage, error) {
	d, err := decodeDraft(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(d.Common)
}
func (*Task) ApplyCommonSettings(raw []byte, common json.RawMessage) ([]byte, string, error) {
	d, err := decodeDraft(raw)
	if err != nil {
		return nil, "", err
	}
	d.Common = Common{} // Common settings replace the group; omitted switches are off.
	dec := json.NewDecoder(bytes.NewReader(common))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&d.Common); err != nil {
		return nil, "", invalid(err.Error())
	}
	if err = validateCommon(d.Common); err != nil {
		return nil, "", err
	}
	b, err := json.Marshal(d)
	return b, workset.CanonicalJSONHash(b), err
}
func (*Task) ValidateDraft(raw []byte, members []*workset.WorksetMember) error {
	d, err := decodeDraft(raw)
	if err != nil {
		return err
	}
	if err = validateCommon(d.Common); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, m := range members {
		known[m.MemberID] = true
	}
	seen := map[string]bool{}
	for _, m := range d.Members {
		if !known[m.MemberID] || seen[m.MemberID] {
			return invalid("unknown or duplicate member")
		}
		if m.Name != strings.TrimSpace(m.Name) {
			return invalid("member name has surrounding whitespace")
		}
		seen[m.MemberID] = true
	}
	return nil
}
func (t *Task) NormalizeDraft(raw []byte, members []*workset.WorksetMember) ([]byte, string, int, error) {
	if err := t.ValidateDraft(raw, members); err != nil {
		return nil, "", 0, err
	}
	d, _ := decodeDraft(raw)
	d.Members = slices.DeleteFunc(d.Members, func(m Member) bool { return !m.Excluded && m.Name == "" })
	if d.Members == nil {
		d.Members = []Member{}
	}
	slices.SortFunc(d.Members, func(a, b Member) int { return strings.Compare(a.MemberID, b.MemberID) })
	b, err := json.Marshal(d)
	return b, workset.CanonicalJSONHash(b), 1, err
}
func (t *Task) ValidateSessionInput(raw []byte, members []*workset.WorksetMember) error {
	return t.ValidateDraft(raw, members)
}

func tokens(template string) ([]string, error) {
	fields := []string{}
	for len(template) > 0 {
		i := strings.IndexAny(template, "{}")
		if i < 0 {
			break
		}
		if template[i] != '{' {
			return nil, invalid("unmatched template brace")
		}
		end := strings.IndexByte(template[i+1:], '}')
		if end < 0 {
			return nil, invalid("unmatched template brace")
		}
		key := template[i+1 : i+1+end]
		switch key {
		case "rj", "title", "circle", "voice_actors", "language", "release_date":
		default:
			return nil, invalid(fmt.Sprintf("unknown placeholder %q", key))
		}
		fields = append(fields, key)
		template = template[i+end+2:]
	}
	return fields, nil
}
func validateTemplate(template string) error {
	fields, err := tokens(template)
	if err != nil {
		return err
	}
	count := 0
	for _, key := range fields {
		if key == "rj" {
			count++
		}
	}
	if count != 1 {
		return invalid("template requires exactly one {rj}")
	}
	return nil
}
