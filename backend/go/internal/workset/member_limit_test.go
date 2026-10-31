package workset_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestCreateCurrentAcceptsMeasuredMemberLimit(t *testing.T) {
	f := newFixture(t)
	paths := make([]string, workset.MaxMembers)
	for i := range paths {
		paths[i] = "album-" + strconv.Itoa(i)
	}
	f.standardLibrary(paths...)
	result, err := f.svc.CreateCurrentWorkset(f.ctx, workset.CreateCurrentRequest{
		LibraryID: "lib-1", OperationType: workset.OperationTypeConversion, Selection: workset.Selection{All: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	limit := workset.MaxMembers
	if result.Recorded != limit || len(result.Workset.Members) != limit || len(result.Skipped) != 0 {
		t.Fatalf("recorded=%d members=%d skipped=%v", result.Recorded, len(result.Workset.Members), result.Skipped)
	}
	// Members follow the inventory's path order, not the order they were created in.
	sorted := slices.Sorted(slices.Values(paths))
	for i, member := range result.Workset.Members {
		if member.RelPath != sorted[i] {
			t.Fatalf("member %d path=%q, want %q", i, member.RelPath, sorted[i])
		}
	}
}
