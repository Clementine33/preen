package librarypath_test

import (
	"testing"

	"github.com/onsei/organizer/backend/internal/librarypath"
)

func TestNormalizeToPOSIX(t *testing.T) {
	got := librarypath.NormalizeToPOSIX(`C:\music\A\B.mp3`)
	if got != "C:/music/A/B.mp3" {
		t.Fatalf("got %q", got)
	}
}

func TestCleanRootPathAndRootPathKey(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantClean string
		wantKey   string
	}{
		{name: "posix trailing", in: "/music/", wantClean: "/music", wantKey: "/music"},
		{name: "posix dot", in: "/music/.", wantClean: "/music", wantKey: "/music"},
		{name: "posix repeated separators", in: "/music//A//", wantClean: "/music/A", wantKey: "/music/A"},
		{name: "posix root", in: "/", wantClean: "/", wantKey: "/"},
		{name: "posix case preserved", in: "/Music", wantClean: "/Music", wantKey: "/Music"},
		{name: "posix backslash", in: `\music\x`, wantClean: "/music/x", wantKey: "/music/x"},
		{name: "windows drive preserves case display", in: `C:\Music\`, wantClean: "C:/Music", wantKey: "c:/music"},
		{name: "windows drive lower", in: "c:/music", wantClean: "c:/music", wantKey: "c:/music"},
		{name: "windows drive root", in: "C:/", wantClean: "C:/", wantKey: "c:/"},
		{
			name:      "windows drive case variant collides",
			in:        `C:\MUSIC\Album`,
			wantClean: "C:/MUSIC/Album",
			wantKey:   "c:/music/album",
		},
		{name: "unc case folded", in: `\\SERVER\Share\Dir\..\`, wantClean: "//SERVER/Share", wantKey: "//server/share"},
		{name: "device path", in: `\\?\C:\music`, wantClean: `//?/C:/music`, wantKey: `//?/c:/music`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := librarypath.CleanRootPath(tt.in); got != tt.wantClean {
				t.Errorf("pathnorm.CleanRootPath(%q) = %q, want %q", tt.in, got, tt.wantClean)
			}
			if got := librarypath.RootPathKey(tt.in); got != tt.wantKey {
				t.Errorf("pathnorm.RootPathKey(%q) = %q, want %q", tt.in, got, tt.wantKey)
			}
		})
	}
}

func TestIsWindowsUNCPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "standard unc", path: `\\server\share\music`, want: true},
		{name: "long unc", path: `\\?\UNC\server\share\music`, want: true},
		{name: "long local drive", path: `\\?\C:\\music`, want: false},
		{name: "drive path", path: `C:\\music`, want: false},
		{name: "slash unc", path: `//server/share/music`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := librarypath.IsWindowsUNCPath(tt.path)
			if got != tt.want {
				t.Fatalf("pathnorm.IsWindowsUNCPath(%q)=%v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsWithinRoot(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		candidate string
		want      bool
	}{
		{name: "same POSIX path", root: "/music", candidate: "/music", want: true},
		{name: "POSIX descendant", root: "/music", candidate: "/music/album/track.flac", want: true},
		{name: "rejects sibling prefix", root: "/music", candidate: "/music-other/track.flac", want: false},
		{name: "cleans parent traversal", root: "/music", candidate: "/music/album/../track.flac", want: true},
		{name: "rejects escaped traversal", root: "/music", candidate: "/music/../outside.flac", want: false},
		{name: "Windows separators and case", root: `C:\Music`, candidate: `c:\music\Album\track.flac`, want: true},
		{name: "same Windows drive root", root: `C:\`, candidate: `c:/`, want: true},
		{name: "rejects another Windows drive", root: `C:\Music`, candidate: `D:\Music\track.flac`, want: false},
		{
			name:      "UNC comparison is case insensitive",
			root:      `\\Server\Share\Music`,
			candidate: `\\server\share\music\track.flac`,
			want:      true,
		},
		{name: "empty candidate", root: "/music", candidate: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := librarypath.IsWithinRoot(tt.root, tt.candidate); got != tt.want {
				t.Fatalf("pathnorm.IsWithinRoot(%q, %q) = %v, want %v", tt.root, tt.candidate, got, tt.want)
			}
		})
	}
}

// TestRelPath is the containment contract every scoped route rests on: a
// relative path is a plain descendant chain, and everything that could name
// something outside the root it will be joined to is refused.
func TestRelPath(t *testing.T) {
	accepted := map[string]string{
		"albumA":          "albumA",
		"albumA/disc2":    "albumA/disc2",
		"albumA/disc2/01": "albumA/disc2/01",
		"with space/日本語":  "with space/日本語",
		"100%_hits":       "100%_hits",
	}
	for input, want := range accepted {
		got, ok := librarypath.RelPath(input)
		if !ok || got != want {
			t.Errorf("RelPath(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}

	refused := []string{
		"", ".", "..", "./x", "../x", "x/../y", "x//y", "/x", "/", "//server/share",
		`x\y`, "C:/music", "x/", "x/./y", "a\x00b",
	}
	for _, input := range refused {
		if got, ok := librarypath.RelPath(input); ok {
			t.Errorf("RelPath(%q) = %q, want refused", input, got)
		}
	}
}

// TestJoinRel pins the join: cleaned POSIX form, root trailing slashes
// tolerated, always a descendant of the root.
func TestJoinRel(t *testing.T) {
	cases := []struct{ root, rel, want string }{
		{"/music", "albumA", "/music/albumA"},
		{"/music/", "albumA", "/music/albumA"},
		{`C:\music`, "albumA", "C:/music/albumA"},
		{"/music", "albumA/disc2", "/music/albumA/disc2"},
		{"C:/", "分类/RJ000001", "C:/分类/RJ000001"},
		{`\\SERVER\Share`, "分类/RJ000001", "//SERVER/Share/分类/RJ000001"},
		{`\\?\C:\music`, "分类/RJ000001", "//?/C:/music/分类/RJ000001"},
	}
	for _, tc := range cases {
		if got := librarypath.JoinRel(tc.root, tc.rel); got != tc.want {
			t.Errorf("JoinRel(%q, %q) = %q, want %q", tc.root, tc.rel, got, tc.want)
		}
		if !librarypath.IsWithinRoot(tc.root, librarypath.JoinRel(tc.root, tc.rel)) {
			t.Errorf("JoinRel(%q, %q) left the root", tc.root, tc.rel)
		}
	}
}
