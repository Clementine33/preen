package updatecheck

import (
	"strings"

	"golang.org/x/mod/semver"
)

// Versions are compared with golang.org/x/mod/semver, which requires the
// leading "v" that build stamps such as "0.2.0" leave out.
func canonical(raw string) string { return "v" + strings.TrimPrefix(raw, "v") }

func validSemver(raw string) bool { return semver.IsValid(canonical(raw)) }

// compareSemver orders two valid versions by semver precedence: a release is
// newer than any of its pre-releases, so 0.2.0 > 0.2.0-rc.2.
func compareSemver(a, b string) int { return semver.Compare(canonical(a), canonical(b)) }
