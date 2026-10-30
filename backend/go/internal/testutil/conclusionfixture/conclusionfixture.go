// Package conclusionfixture checks the member conclusions stored at generation
// against the rule re-derived from the reloaded full plan.
package conclusionfixture

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// AssertStored requires one stored conclusion per record member, each equal to
// what the task derives from the member's own frozen root and components. It
// returns the stored conclusions keyed by member.
func AssertStored(
	t *testing.T, store workset.Store, task workset.Task, worksetID, operationType, planID string,
) map[string]workset.MemberConclusion {
	t.Helper()
	members, err := store.ListWorksetMembers(worksetID)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := store.GetOperationRevision(worksetID, operationType, planID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetPlanDetail(planID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.ListMemberConclusions(planID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(members) {
		t.Fatalf("stored %d conclusions for %d members", len(stored), len(members))
	}
	byMember := make(map[string]workset.MemberConclusion, len(stored))
	for _, c := range stored {
		byMember[c.MemberID] = c
	}
	provider, provides := task.(workset.MemberConclusionTask)
	for _, member := range members {
		got, ok := byMember[member.MemberID]
		if !ok {
			t.Fatalf("member %s has no stored conclusion", member.MemberID)
		}
		excluded := strings.Contains("\x00"+rev.ExcludedScope+"\x00", "\x00"+member.MemberID+"\x00")
		var owned []json.RawMessage
		if !excluded {
			for _, root := range detail.Roots {
				if root.MemberID != member.MemberID {
					continue
				}
				for _, component := range detail.Components {
					if component.RootIndex == root.RootIndex {
						owned = append(owned, json.RawMessage(component.OutcomeJSON))
					}
				}
				break
			}
		}
		var payload json.RawMessage
		if provides {
			if payload, err = provider.MemberConclusion(owned); err != nil {
				t.Fatal(err)
			}
		}
		if got.Excluded != excluded || got.HasComponents != (len(owned) > 0) || !bytes.Equal(got.Task, payload) ||
			got.Input != "" {
			t.Fatalf("member %s stored %+v, derived excluded=%v components=%d task=%s",
				member.MemberID, got, excluded, len(owned), payload)
		}
	}
	return byMember
}
