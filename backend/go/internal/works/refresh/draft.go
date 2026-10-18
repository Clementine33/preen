package refresh

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/Clementine33/preen/backend/internal/workset"
)

type Common struct {
	Mode          Mode   `json:"mode"`
	Locale        string `json:"locale"`
	DownloadCover bool   `json:"download_cover"`
}
type Member struct {
	MemberID string `json:"member_id"`
	Excluded bool   `json:"excluded"`
}
type Draft struct {
	SchemaVersion int      `json:"schema_version"`
	Common        Common   `json:"common"`
	Members       []Member `json:"members"`
}

func defaults() Common {
	return Common{Mode: FillMissing, Locale: "ja-jp", DownloadCover: true}
}
func validateCommon(c Common) error {
	if c.Mode != FillMissing && c.Mode != RefreshAll && c.Mode != RefreshDynamic {
		return invalid("unsupported mode")
	}
	if c.Locale != "ja-jp" && c.Locale != "zh-cn" && c.Locale != "zh-tw" {
		return invalid("unsupported locale")
	}
	return nil
}
func invalid(msg string) error {
	return workset.NewError(workset.ErrKindInvalidArgument, "INVALID_DRAFT", msg, nil)
}
func decode(raw []byte) (Draft, error) {
	d := Draft{Common: defaults(), Members: []Member{}}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, invalid(err.Error())
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return d, invalid("trailing content")
	}
	if d.SchemaVersion != 1 {
		return d, workset.NewError(
			workset.ErrKindInvalidArgument,
			"UNSUPPORTED_SCHEMA_VERSION",
			"unsupported metadata draft",
			nil,
		)
	}
	if d.Common.Mode == RefreshDynamic {
		d.Common.DownloadCover = false
	}
	return d, validateCommon(d.Common)
}
func (*Task) SeedDraft(common json.RawMessage) ([]byte, string, int) {
	d := Draft{SchemaVersion: 1, Common: defaults(), Members: []Member{}}
	if len(common) > 0 {
		c := defaults()
		if json.Unmarshal(common, &c) == nil && validateCommon(c) == nil {
			d.Common = c
		}
	}
	if d.Common.Mode == RefreshDynamic {
		d.Common.DownloadCover = false
	}
	raw, _ := json.Marshal(d)
	return raw, workset.CanonicalJSONHash(raw), 1
}
func (*Task) CommonSettings(raw []byte) (json.RawMessage, error) {
	d, err := decode(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(d.Common)
}
func (*Task) ApplyCommonSettings(raw []byte, common json.RawMessage) ([]byte, string, error) {
	d, err := decode(raw)
	if err != nil {
		return nil, "", err
	}
	dec := json.NewDecoder(bytes.NewReader(common))
	dec.DisallowUnknownFields()
	c := defaults()
	if err = dec.Decode(&c); err != nil {
		return nil, "", invalid(err.Error())
	}
	if err = dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, "", invalid("trailing content")
	}
	if err = validateCommon(c); err != nil {
		return nil, "", err
	}
	if c.Mode == RefreshDynamic {
		c.DownloadCover = false
	}
	d.Common = c
	b, err := json.Marshal(d)
	return b, workset.CanonicalJSONHash(b), err
}
func (*Task) ValidateDraft(raw []byte, members []*workset.WorksetMember) error {
	d, err := decode(raw)
	if err != nil {
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
		seen[m.MemberID] = true
	}
	return nil
}
func (t *Task) NormalizeDraft(raw []byte, members []*workset.WorksetMember) ([]byte, string, int, error) {
	if err := t.ValidateDraft(raw, members); err != nil {
		return nil, "", 0, err
	}
	d, _ := decode(raw)
	d.Members = slices.DeleteFunc(d.Members, func(m Member) bool { return !m.Excluded })
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
