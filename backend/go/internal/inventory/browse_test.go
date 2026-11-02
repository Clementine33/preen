package inventory_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/inventory"
)

// TestBuildMemberTreeAcceptsANativeMemberRoot covers a Windows caller: the
// member path arrives in the host's spelling (backslashes) while the entries
// carry the stored POSIX form, so the root must be normalized before children
// are attached — otherwise every node is dropped and the folder reads empty.
func TestBuildMemberTreeAcceptsANativeMemberRoot(t *testing.T) {
	const stored = "C:/music/albumA"
	entries := []inventory.Entry{
		{Path: stored + "/disc2", ParentPath: stored, Name: "disc2", IsDir: true},
		{Path: stored + "/01.flac", ParentPath: stored, Name: "01.flac", Size: 1234, Format: "flac"},
		{Path: stored + "/disc2/02.flac", ParentPath: stored + "/disc2", Name: "02.flac", Size: 2048, Format: "flac"},
	}
	tree := inventory.BuildMemberTree(`C:\music\albumA`, entries)

	if tree.Name != "albumA" || tree.Path != stored || tree.RelPath != "" {
		t.Fatalf("root = %q / %q / %q, want albumA / %q / \"\"", tree.Name, tree.Path, tree.RelPath, stored)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("root children = %d, want 2 (the folder read empty)", len(tree.Children))
	}
	// Dirs sort before files, and the nested file keeps its member-relative path.
	if first, second := tree.Children[0], tree.Children[1]; first.Name != "disc2" || second.Name != "01.flac" {
		t.Fatalf("children = %q, %q, want disc2, 01.flac", first.Name, second.Name)
	}
	if nested := tree.Children[0].Children; len(nested) != 1 || nested[0].RelPath != "disc2/02.flac" {
		t.Fatalf("disc2 children = %+v, want one node with rel_path disc2/02.flac", nested)
	}
}
