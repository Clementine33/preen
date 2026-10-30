package workset

import "encoding/json"

// concludeRevisionMembers resolves frozen root ownership once for all members.
// Task payloads remain opaque; the task receives only one member's components.
func concludeRevisionMembers(members []RevisionMember, detail *PlanDetail, staleRoots map[int]bool, task Task) error {
	roots := make(map[string]PlanRootRecord, len(detail.Roots))
	for _, root := range detail.Roots {
		if root.MemberID == "" {
			continue
		}
		if _, exists := roots[root.MemberID]; !exists {
			roots[root.MemberID] = root
		}
	}
	components := make(map[int][]json.RawMessage)
	for _, component := range detail.Components {
		components[component.RootIndex] = append(
			components[component.RootIndex],
			json.RawMessage(component.OutcomeJSON),
		)
	}
	provider, provides := task.(MemberConclusionTask)
	for i := range members {
		member := &members[i]
		conclusion := MemberConclusion{MemberID: member.MemberID, Excluded: member.Excluded, Input: "none"}
		root, exists := roots[member.MemberID]
		var owned []json.RawMessage
		if exists && !member.Excluded {
			owned = components[root.RootIndex]
			conclusion.HasComponents = len(owned) > 0
			switch {
			case root.RootStatus == "missing":
				conclusion.Input = "missing"
			case staleRoots[root.RootIndex]:
				conclusion.Input = "stale"
			default:
				conclusion.Input = "valid"
			}
		}
		if provides {
			payload, err := provider.MemberConclusion(owned)
			if err != nil {
				return err
			}
			conclusion.Task = payload
		}
		member.Conclusion = conclusion
	}
	return nil
}
