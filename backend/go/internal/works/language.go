package works

import "slices"

// PreferredLanguage selects one declared work language, preferring simplified
// Chinese, traditional Chinese and Japanese. DOT denotes translation provenance.
func PreferredLanguage(codes []string) *string {
	for _, code := range []string{
		"CHI_HANS", "CHI_HANT", "JPN", "ENG", "CHI", "KO_KR", "SPA", "GER", "FRE",
		"IND", "ITA", "POR", "SWE", "THA", "VIE", "RUS", "OTL", "NM",
	} {
		if slices.Contains(codes, code) {
			return &code
		}
	}
	return nil
}
