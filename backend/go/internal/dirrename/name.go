package dirrename

import (
	"path/filepath"
	"strings"

	"github.com/onsei/organizer/backend/internal/librarypath"
	"github.com/onsei/organizer/backend/internal/works"
	"github.com/onsei/organizer/backend/internal/workset"
)

func planName(template string, m *workset.WorksetMember, work Work, entries Directory, root string) Outcome {
	o := Outcome{
		MemberID:   m.MemberID,
		SourceName: filepath.Base(filepath.FromSlash(m.FolderPath)),
		WorkCode:   work.Code,
		Action:     "blocked",
	}
	entry, exists := entries[o.SourceName]
	if !exists {
		o.ReasonCode = "SOURCE_MISSING"
		return o
	}
	if !entry.Directory || entry.Symlink {
		o.ReasonCode = "SOURCE_NOT_DIRECTORY"
		return o
	}
	if work.Code == "" {
		o.ReasonCode = "WORK_UNBOUND"
		return o
	}
	fields, _ := tokens(template)
	replacements := []string{}
	for _, key := range fields {
		value := work.Values[key]
		if key == "rj" {
			value = work.Code
		}
		if strings.TrimSpace(value) == "" {
			o.ReasonCode = "METADATA_MISSING"
			return o
		}
		replacements = append(replacements, "{"+key+"}", value)
	}
	o.RawName = strings.NewReplacer(replacements...).Replace(template)
	o.TargetName = strings.TrimRight(strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune("<>:\"/\\|?*", r) {
			return '_'
		}
		return r
	}, o.RawName)), " .")
	if len(o.TargetName) > 255 {
		o.ReasonCode = "NAME_TOO_LONG"
		return o
	}
	if reserved(o.TargetName) {
		o.ReasonCode = "NAME_RESERVED"
		return o
	}
	codes := works.CodesInDirName(o.TargetName)
	if codes.Kind != works.DirCodeSingle || string(codes.Code) != work.Code {
		o.ReasonCode = "WORK_CODE_AMBIGUOUS"
		return o
	}
	if o.TargetName == o.SourceName {
		o.Action = "noop"
		return o
	}
	targetKey := librarypath.RootPathKey(filepath.ToSlash(filepath.Join(root, o.TargetName)))
	for name := range entries {
		if librarypath.RootPathKey(filepath.ToSlash(filepath.Join(root, name))) == targetKey {
			o.ReasonCode = "TARGET_EXISTS"
			if name == o.SourceName {
				o.ReasonCode = "CASE_ONLY_RENAME"
			}
			return o
		}
	}
	o.Action = "rename"
	return o
}
func reserved(name string) bool {
	if name == "" || name == "." || name == ".." {
		return true
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' &&
		base[3] <= '9'
}
