package dirrename

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/onsei/organizer/backend/internal/librarypath"
	"github.com/onsei/organizer/backend/internal/works"
	"github.com/onsei/organizer/backend/internal/workset"
)

var bracketedTitle = regexp.MustCompile(`(?s)【.*?】`)
var tildeTitle = regexp.MustCompile(`(?s)~.*?~|～.*?～`)
var bookTitle = regexp.MustCompile(`(?s)《.*?》`)

// naming carries the per-folder inputs of planName that the shared template
// does not: the member's own name and the directory a hand rename left behind.
type naming struct {
	// Custom replaces the template with a literal name chosen for this folder.
	Custom string
	// Observed is the name the recorded folder now has on disk after a hand
	// rename; empty when the recorded name still exists.
	Observed string
}

// followed finds the directory a hand rename left behind: the single directory
// in the same parent that carries the recorded folder's work code and is not
// another member's. None or several leave the member missing.
func followed(recorded string, entries Directory, taken map[string]bool) string {
	if _, exists := entries[recorded]; exists {
		return ""
	}
	want := works.CodesInDirName(recorded)
	if want.Kind != works.DirCodeSingle {
		return ""
	}
	found := ""
	for name, entry := range entries {
		if !entry.Directory || entry.Symlink || taken[name] {
			continue
		}
		if got := works.CodesInDirName(name); got.Kind == works.DirCodeSingle && got.Code == want.Code {
			if found != "" {
				return ""
			}
			found = name
		}
	}
	return found
}

func planName(
	common Common, m *workset.WorksetMember, work Work, entries Directory, root string, n naming,
) Outcome {
	template := common.Template
	o := Outcome{
		MemberID:     m.MemberID,
		SourceName:   filepath.Base(filepath.FromSlash(m.FolderPath)),
		WorkCode:     work.Code,
		Action:       "blocked",
		CustomName:   n.Custom,
		ObservedName: n.Observed,
	}
	disk := o.SourceName
	if n.Observed != "" {
		disk = n.Observed
	}
	entry, exists := entries[disk]
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
	if n.Custom != "" {
		o.RawName = n.Custom
	} else {
		fields, _ := tokens(template)
		replacements := []string{}
		for _, key := range fields {
			value := work.field(key, common)
			if key == "title" {
				if common.StripBracketedTitle {
					value = bracketedTitle.ReplaceAllString(value, "")
				}
				if common.StripTildeTitle {
					value = tildeTitle.ReplaceAllString(value, "")
				}
				if common.StripBookTitle {
					value = bookTitle.ReplaceAllString(value, "")
				}
			}
			if strings.TrimSpace(value) == "" {
				o.ReasonCode = "METADATA_MISSING"
				return o
			}
			replacements = append(replacements, "{"+key+"}", value)
		}
		o.RawName = strings.NewReplacer(replacements...).Replace(template)
	}
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
	switch {
	case n.Custom != "" && codes.Kind != works.DirCodeSingle:
		o.ReasonCode = "NAME_CODE_INVALID"
		return o
	case n.Custom != "" && string(codes.Code) != work.Code:
		// A name that carries another work's code re-binds the folder to it.
		o.RebindTo = string(codes.Code)
	case n.Custom == "" && (codes.Kind != works.DirCodeSingle || string(codes.Code) != work.Code):
		o.ReasonCode = "WORK_CODE_AMBIGUOUS"
		return o
	}
	if o.TargetName == disk {
		o.Action = "noop"
		return o
	}
	targetKey := librarypath.RootPathKey(filepath.ToSlash(filepath.Join(root, o.TargetName)))
	for name := range entries {
		if librarypath.RootPathKey(filepath.ToSlash(filepath.Join(root, name))) == targetKey {
			o.ReasonCode = "TARGET_EXISTS"
			if name == disk {
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
