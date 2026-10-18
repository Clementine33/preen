package librarypath_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/librarypath"
)

func TestIsRecoveryRelPath(t *testing.T) {
	for _, tc := range []struct {
		root string
		rel  string
		want bool
	}{
		{"/music", "Delete", true},
		{"/music", "分类/Delete/RJ000001", true},
		{"/music", "分类/RJ000001/Delete/track.mp3", true},
		{"/music", "分类/delete/RJ000001", false},
		{"/music", "Delete extras/RJ000001", false},
		{"C:/Music", "分类/delete/RJ000001", true},
		{"//server/share", "分类/DELETE/RJ000001", true},
	} {
		t.Run(tc.root+"/"+tc.rel, func(t *testing.T) {
			if got := librarypath.IsRecoveryRelPath(tc.root, tc.rel); got != tc.want {
				t.Fatalf("IsRecoveryRelPath = %v, want %v", got, tc.want)
			}
		})
	}
}
