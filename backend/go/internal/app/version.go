package app

import "strings"

// IsDevelopmentVersion checks pre-release identifiers, ignoring build metadata.
// RCs use the same migration lifecycle as stable releases (ADR 0011).
func IsDevelopmentVersion(version string) bool {
	version, _, _ = strings.Cut(version, "+")
	_, prerelease, _ := strings.Cut(version, "-")
	for identifier := range strings.SplitSeq(prerelease, ".") {
		if identifier == "dev" {
			return true
		}
	}
	return false
}
