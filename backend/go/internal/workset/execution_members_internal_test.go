// This test is in the package (not workset_test) on purpose: fixtures with more
// than a handful of members do not exist at the exported seam, and the cap and
// order of the exception list are a property of the fold itself.
package workset

import (
	"strconv"
	"testing"
)

func TestMembersViewCapsAttentionInFrozenOrder(t *testing.T) {
	const members = attentionCap + 10
	f := &memberFold{
		owner: map[int]int{}, byComp: map[string]int{}, byMember: map[string]int{},
		digests: map[int]ExecutionResultDigest{}, running: -1,
	}
	for i := range members {
		id := "m" + strconv.Itoa(i)
		f.recs = append(f.recs, memberRec{id: id, root: "/" + id, idx: []int{i}})
		f.owner[i] = i
		f.digests[i] = ExecutionResultDigest{ComponentIndex: i, Status: ExecComponentFailed, ErrorCode: "X"}
		f.sums = append(f.sums, f.summarize(i))
		f.account(f.sums[i], 1)
	}
	view := f.view()
	if view.AttentionTotal != members || len(view.Attention) != attentionCap || view.Counts.Failed != members {
		t.Fatalf("total %d, listed %d, counts %+v", view.AttentionTotal, len(view.Attention), view.Counts)
	}
	for i, m := range view.Attention {
		if m.MemberID != "m"+strconv.Itoa(i) {
			t.Fatalf("attention[%d] = %s", i, m.MemberID)
		}
	}
}
