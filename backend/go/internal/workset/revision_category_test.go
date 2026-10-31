package workset_test

import "testing"

func TestRevisionCategoryReadsAreCountedFilteredAndFrozenOrdered(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("a", "b", "c", "d")
	ws := f.createCurrent("类别", ids...)
	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" {
		t.Fatalf("generation: %+v", gen)
	}
	// Conversion declares no category: nothing is counted.
	plain, err := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID)
	if err != nil || len(plain.CategoryCounts) != 0 || plain.Category != "" {
		t.Fatalf("uncategorised revision = %+v, %v", plain.CategoryCounts, err)
	}
	// Categories are opaque to workset; file members 0,2,3 under "x" and 1 under "y".
	for i, member := range ws.Members {
		category := "x"
		if i == 1 {
			category = "y"
		}
		f.exec("UPDATE plan_member_conclusions SET category = ? WHERE plan_id = ? AND member_id = ?",
			category, gen.RevisionID, member.MemberID)
	}
	full, err := f.svc.GetRevision(f.ctx, ws.WorksetID, "conversion", gen.RevisionID)
	if err != nil || full.CategoryCounts["x"] != 3 || full.CategoryCounts["y"] != 1 || len(full.Members) != 4 {
		t.Fatalf("full revision = %+v, %v", full.CategoryCounts, err)
	}
	limited, err := f.svc.GetRevisionCategory(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, "x", 2)
	if err != nil {
		t.Fatal(err)
	}
	if limited.Category != "x" || limited.CategoryTotal != 3 || len(limited.Members) != 2 ||
		limited.Members[0].MemberID != ws.Members[0].MemberID ||
		limited.Members[1].MemberID != ws.Members[2].MemberID {
		t.Fatalf("limited = total %d members %+v", limited.CategoryTotal, limited.Members)
	}
	if limited.CategoryCounts["x"] != 3 || limited.CategoryCounts["y"] != 1 {
		t.Fatalf("limited counts = %+v", limited.CategoryCounts)
	}
	none, err := f.svc.GetRevisionCategory(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, "absent", 5)
	if err != nil || none.CategoryTotal != 0 || len(none.Members) != 0 {
		t.Fatalf("absent category = %+v, %v", none, err)
	}
}
