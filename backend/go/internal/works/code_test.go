package works_test

import (
	"testing"

	"github.com/onsei/organizer/backend/internal/works"
)

func TestParseWorkCodeAcceptsSixAndEightDigits(t *testing.T) {
	cases := []struct {
		in   string
		want works.WorkCode
	}{
		// The same number written two ways is one identity: the six-digit form
		// is zero-padded, so it shares a primary key with the eight-digit one.
		{"RJ123456", "RJ00123456"},
		{"RJ00123456", "RJ00123456"},
		{"rj01657200", "RJ01657200"},
		{"RJ01657200", "RJ01657200"},
		{"rj012345", "RJ00012345"},
	}
	for _, tc := range cases {
		got, ok := works.ParseWorkCode(tc.in)
		if !ok {
			t.Errorf("ParseWorkCode(%q) refused a legal code", tc.in)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseWorkCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if again := got.Canonical(); again != got {
			t.Errorf("Canonical(%q) = %q, want it unchanged", got, again)
		}
	}
}

func TestParseWorkCodeRefusesEverythingElse(t *testing.T) {
	// 7 digits are not an accepted width even though the reference
	// implementation would accept them by padding; this build refuses them.
	for _, in := range []string{
		"", "RJ", "RJ12345", "RJ01234", "RJ1657200", "RJ012345678",
		"BJ01234567", "VJ01234567", "CC01234567", "RG01038998",
		"RJ00000000", "RJ000000", "RJ01234X67", "xRJ01234567",
		"RJ01234567X", " RJ01234567", "RJ-01234567",
	} {
		if got, ok := works.ParseWorkCode(in); ok {
			t.Errorf("ParseWorkCode(%q) = %q, want a refusal", in, got)
		}
	}
}

func TestCodesInDirName(t *testing.T) {
	single := func(code works.WorkCode) works.DirCodeResult {
		return works.DirCodeResult{Kind: works.DirCodeSingle, Code: code}
	}
	multiple := func(codes ...works.WorkCode) works.DirCodeResult {
		return works.DirCodeResult{Kind: works.DirCodeMultiple, Codes: codes}
	}
	singleWithStray := func(code works.WorkCode, invalid int) works.DirCodeResult {
		return works.DirCodeResult{
			Kind:         works.DirCodeMultiple,
			Codes:        []works.WorkCode{code},
			InvalidCount: invalid,
		}
	}
	invalidOnly := func(invalid int) works.DirCodeResult {
		return works.DirCodeResult{Kind: works.DirCodeNone, InvalidCount: invalid}
	}

	cases := []struct {
		name string
		in   string
		want works.DirCodeResult
	}{
		{"bare eight digits", "RJ01234567", single("RJ01234567")},
		{"bare six digits are legal", "RJ012345", single("RJ00012345")},
		{"title around the number", "【ASMR】RJ01234567 耳かき", single("RJ01234567")},
		{"underscore separates", "RJ01234567_extra", single("RJ01234567")},

		// The adjacent case: neither candidate has a separator on both sides,
		// so neither is a token at all and the name carries no number. It must
		// not be read as a single (wrong binding) or as multiple.
		{"adjacent digits, no separator", "RJ0123456RJ01234567", works.DirCodeResult{Kind: works.DirCodeNone}},

		// Two distinct numbers, either separator, must both be found.
		{"space separated pair", "RJ01234567 RJ07654321", multiple("RJ01234567", "RJ07654321")},
		{"hyphen separated pair", "RJ01234567-RJ07654321", multiple("RJ01234567", "RJ07654321")},
		{"the same number twice is one number", "RJ01234567 RJ01234567", single("RJ01234567")},
		{"same number in both spellings", "RJ123456 RJ00123456", single("RJ00123456")},

		// A malformed candidate beside a legal number is ambiguous: refuse to
		// guess which one the name meant.
		{"malformed candidate beside a number", "RJ0123456 RJ01234567", singleWithStray("RJ01234567", 1)},
		{"nine-digit run is not truncated", "RJ012345678", invalidOnly(1)},
		{"five-digit run", "RJ01234", invalidOnly(1)},
		{"zero number", "RJ00000000", invalidOnly(1)},

		{"weld to a letter", "XRJ01234567", works.DirCodeResult{Kind: works.DirCodeNone}},
		{"weld to a trailing letter", "RJ01234567X", works.DirCodeResult{Kind: works.DirCodeNone}},
		{"no number at all", "some album name", works.DirCodeResult{Kind: works.DirCodeNone}},
		{"no prefix", "01234567", works.DirCodeResult{Kind: works.DirCodeNone}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := works.CodesInDirName(tc.in)
			if got.Kind != tc.want.Kind {
				t.Fatalf("CodesInDirName(%q).Kind = %d, want %d", tc.in, got.Kind, tc.want.Kind)
			}
			if got.Code != tc.want.Code {
				t.Errorf("CodesInDirName(%q).Code = %q, want %q", tc.in, got.Code, tc.want.Code)
			}
			if got.InvalidCount != tc.want.InvalidCount {
				t.Errorf("CodesInDirName(%q).InvalidCount = %d, want %d", tc.in, got.InvalidCount, tc.want.InvalidCount)
			}
			if len(got.Codes) != len(tc.want.Codes) {
				t.Fatalf("CodesInDirName(%q).Codes = %v, want %v", tc.in, got.Codes, tc.want.Codes)
			}
			for i := range got.Codes {
				if got.Codes[i] != tc.want.Codes[i] {
					t.Errorf("CodesInDirName(%q).Codes[%d] = %q, want %q", tc.in, i, got.Codes[i], tc.want.Codes[i])
				}
			}
		})
	}
}
