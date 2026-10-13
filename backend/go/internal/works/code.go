// Package works owns the logical work catalog: the work numbers as immutable
// identities, the source metadata of a work, the relations between an original
// and its translations, and the single user's data about a work. The physical
// copies of a work inside a media library bind to it from here; the file facts
// themselves stay with package inventory.
// Ordered collections belong to package collections; this catalog exposes
// Favorites membership as personal data and protects referenced works on deletion.
//
// Storage arrives as a consumer-defined port (Store) implemented by the SQLite
// adapter.
package works

import (
	"regexp"
	"strings"
)

// WorkCode is a canonical work number: "RJ" followed by exactly eight decimal
// digits, zero-padded. Callers never assemble one by hand — ParseWorkCode is
// the only constructor — so a stored id is always in this one form and the
// same number never gets two primary keys.
type WorkCode string

// codePrefix is the only accepted number prefix.
const codePrefix = "RJ"

// canonicalDigits is the fixed width of a canonical number: six-digit inputs
// are zero-padded to it so RJ123456 and RJ00123456 share one identity.
const canonicalDigits = 8

// acceptedDigitWidths are the lengths the format allows, before padding.
var acceptedDigitWidths = [2]int{6, canonicalDigits}

// ParseWorkCode reads one work number: the prefix in either case followed by
// exactly six or eight decimal digits whose value is not zero. Anything else —
// another prefix (BJ/VJ/CC/RG), a seven- or nine-digit run, or trailing
// garbage — is refused. The result is already canonical, so it can be used as
// a database key directly.
func ParseWorkCode(s string) (WorkCode, bool) {
	if len(s) < len(codePrefix)+acceptedDigitWidths[0] {
		return "", false
	}
	if !strings.EqualFold(s[:len(codePrefix)], codePrefix) {
		return "", false
	}
	digits := s[len(codePrefix):]
	if !validDigits(digits) {
		return "", false
	}
	return WorkCode(codePrefix + strings.Repeat("0", canonicalDigits-len(digits)) + digits), true
}

// Canonical returns the eight-digit zero-padded form. A code produced by
// ParseWorkCode is already canonical, so this is the identity function for
// every value the package hands out; it exists for values that arrived from
// somewhere that only guarantees a valid, unpadded number.
func (c WorkCode) Canonical() WorkCode {
	digits := strings.TrimPrefix(strings.ToUpper(string(c)), codePrefix)
	if !validDigits(digits) || len(digits) == canonicalDigits {
		return c
	}
	return WorkCode(codePrefix + strings.Repeat("0", canonicalDigits-len(digits)) + digits)
}

// validDigits reports whether s is a legal number body: exactly six or eight
// digits, not all zero.
func validDigits(s string) bool {
	if len(s) != acceptedDigitWidths[0] && len(s) != acceptedDigitWidths[1] {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return strings.Trim(s, "0") != ""
}

// DirCodeKind is what a directory name yielded.
type DirCodeKind uint8

const (
	// DirCodeNone means the name holds no work number, or holds candidates that
	// are all malformed: the directory is ordinary inventory with no binding.
	DirCodeNone DirCodeKind = iota
	// DirCodeSingle means exactly one work number and no malformed candidate.
	DirCodeSingle
	// DirCodeMultiple means two or more distinct work numbers, or one number
	// alongside a malformed candidate. Both are ambiguous on purpose: a
	// half-recognized name is not guessed at.
	DirCodeMultiple
)

// DirCodeResult is the extraction outcome. It is a struct rather than a
// (code, ok) pair because the scan reports per-category counts, and "no number"
// and "several numbers" are different categories the caller must be able to
// tell apart.
type DirCodeResult struct {
	Kind DirCodeKind
	// Code is the single recognized number; set when Kind is DirCodeSingle.
	Code WorkCode
	// Codes are the distinct recognized numbers in order of appearance; set
	// when Kind is DirCodeMultiple.
	Codes []WorkCode
	// InvalidCount counts candidate tokens that looked like a number but did
	// not parse (wrong length, zero value).
	InvalidCount int
}

// rjCandidate finds the shape of a number inside a name. The digit run is
// deliberately greedy: a nine-digit run must be read whole and then rejected
// as too long, never truncated to a legal eight-digit prefix. A candidate is
// additionally checked against its neighbours by index below, because RE2 has
// no lookaround.
var rjCandidate = regexp.MustCompile(`(?i)RJ[0-9]+`)

// CodesInDirName extracts the work number a directory name carries. Only the
// name itself is consulted — never file names, parent directories or a
// metadata title — because the directory name is what the user controls.
//
// A match is accepted only when the character before it and the character
// after it are both non-alphanumeric (or the ends of the string). Those two
// characters are inspected by index and never consumed, so two numbers
// separated by one separator are both found; a pattern that carried its own
// boundary would swallow the separator and see only the first of them.
func CodesInDirName(dirName string) DirCodeResult {
	var result DirCodeResult
	seen := map[WorkCode]struct{}{}
	for _, at := range rjCandidate.FindAllStringIndex(dirName, -1) {
		if !atTokenBoundary(dirName, at[0], at[1]) {
			continue
		}
		code, ok := ParseWorkCode(dirName[at[0]:at[1]])
		if !ok {
			result.InvalidCount++
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		result.Codes = append(result.Codes, code)
	}
	switch {
	case len(result.Codes) == 0:
		result.Kind = DirCodeNone
		result.Codes = nil
	case len(result.Codes) == 1 && result.InvalidCount == 0:
		result.Kind = DirCodeSingle
		result.Code = result.Codes[0]
		result.Codes = nil
	default:
		result.Kind = DirCodeMultiple
	}
	return result
}

// atTokenBoundary reports whether the match at [start,end) stands alone: the
// character on each side is a separator rather than another word character.
func atTokenBoundary(s string, start, end int) bool {
	if start > 0 && isWordByte(s[start-1]) {
		return false
	}
	return end >= len(s) || !isWordByte(s[end])
}

// isWordByte reports whether b is an ASCII letter or digit — the characters
// that would weld a number to a neighbouring token.
func isWordByte(b byte) bool {
	switch {
	case b >= '0' && b <= '9', b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z':
		return true
	default:
		return false
	}
}
