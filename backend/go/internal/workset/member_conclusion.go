package workset

import (
	"encoding/json"
	"fmt"
)

// firstRootByMember resolves frozen root ownership: the first root of each member.
func firstRootByMember(roots []PlanRootRecord) map[string]PlanRootRecord {
	out := make(map[string]PlanRootRecord, len(roots))
	for _, root := range roots {
		if root.MemberID == "" {
			continue
		}
		if _, exists := out[root.MemberID]; !exists {
			out[root.MemberID] = root
		}
	}
	return out
}

// freezeMemberConclusions derives every member's conclusion except its Member
// Input from the complete frozen plan: exclusion, whether it owns components
// and the task's opaque payload. It runs once at generation, before staging, so
// readers never decode components to list members (ADR 0001). Task payloads
// remain opaque; the task receives only one member's components.
func freezeMemberConclusions(
	members []RevisionMember, roots []PlanRootRecord, components []PlanComponentRecord, task Task,
) ([]MemberConclusion, error) {
	owners := firstRootByMember(roots)
	byRoot := make(map[int][]json.RawMessage)
	for _, component := range components {
		byRoot[component.RootIndex] = append(byRoot[component.RootIndex], json.RawMessage(component.OutcomeJSON))
	}
	provider, provides := task.(MemberConclusionTask)
	out := make([]MemberConclusion, 0, len(members))
	for _, member := range members {
		conclusion := MemberConclusion{MemberID: member.MemberID, Excluded: member.Excluded}
		var owned []json.RawMessage
		if root, exists := owners[member.MemberID]; exists && !member.Excluded {
			owned = byRoot[root.RootIndex]
			conclusion.HasComponents = len(owned) > 0
		}
		if provides {
			payload, err := provider.MemberConclusion(owned)
			if err != nil {
				return nil, err
			}
			conclusion.Task = payload
		}
		out = append(out, conclusion)
	}
	return out, nil
}

// concludeRevisionMembers joins each member's stored frozen conclusion with its
// live Member Input. Frozen input facts take precedence over live ones: none,
// missing, then unavailable (the record has no library to validate against),
// stale, valid.
func concludeRevisionMembers(
	members []RevisionMember, stored []MemberConclusion, roots []PlanRootRecord,
	unavailable bool, staleRoots map[int]bool,
) error {
	frozen := make(map[string]MemberConclusion, len(stored))
	for _, conclusion := range stored {
		frozen[conclusion.MemberID] = conclusion
	}
	owners := firstRootByMember(roots)
	for i := range members {
		member := &members[i]
		conclusion, ok := frozen[member.MemberID]
		if !ok {
			return fmt.Errorf("member %s has no stored conclusion", member.MemberID)
		}
		conclusion.Input = "none"
		if root, exists := owners[member.MemberID]; exists && !conclusion.Excluded {
			switch {
			case root.RootStatus == "missing":
				conclusion.Input = "missing"
			case unavailable:
				conclusion.Input = "unavailable"
			case staleRoots[root.RootIndex]:
				conclusion.Input = "stale"
			default:
				conclusion.Input = "valid"
			}
		}
		member.Conclusion = conclusion
	}
	return nil
}
