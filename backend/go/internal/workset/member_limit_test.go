package workset_test

import (
	"strconv"
	"testing"

	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestCreateCurrentAcceptsMeasuredMemberLimit(t *testing.T) {
	f := newFixture(t)
	paths := make([]string, 2000)
	for i := range paths {
		paths[i] = "album-" + strconv.Itoa(i)
	}
	f.standardLibrary(paths...)
	result, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID: "lib-1", OperationType: workset.OperationTypeConversion, FolderPaths: paths,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Recorded != 2000 || len(result.Workset.Members) != 2000 || len(result.Skipped) != 0 {
		t.Fatalf("recorded=%d members=%d skipped=%v", result.Recorded, len(result.Workset.Members), result.Skipped)
	}
	for i, member := range result.Workset.Members {
		if member.RelPath != paths[i] {
			t.Fatalf("member %d path=%q, want %q", i, member.RelPath, paths[i])
		}
	}
}
