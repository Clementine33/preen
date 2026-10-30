package conversion

import (
	"encoding/json"

	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

// MemberConclusion summarizes only this member's frozen conversion components.
// Workset owns membership and input health; conversion owns these task facts.
func (*Task) MemberConclusion(components []json.RawMessage) (json.RawMessage, error) {
	var out struct {
		Status    string              `json:"status"`
		Matched   partitionConclusion `json:"matched"`
		Unmatched partitionConclusion `json:"unmatched"`
	}
	blocked, ambiguous, changed, unmetCount := false, false, false, 0
	for _, raw := range components {
		var component reconcile.ComponentOutcome
		if err := json.Unmarshal(raw, &component); err != nil {
			return nil, err
		}
		side := &out.Unmatched
		if component.Partition == "matched" {
			side = &out.Matched
		}
		side.Applicable = true
		side.Blocked = side.Blocked || component.Status == reconcile.StatusBlocked
		side.Changes = side.Changes || len(component.Operations) > 0
		unmet := false
		for _, variant := range component.Variants {
			for _, decision := range variant.Decisions {
				unmet = unmet || decision.ReasonCode == "UNMET_TARGET"
			}
		}
		side.Unmet = side.Unmet || unmet
		if unmet {
			unmetCount++
		}
		blocked = blocked || component.Status == reconcile.StatusBlocked && component.ReasonCode != "SOURCE_AMBIGUOUS"
		ambiguous = ambiguous || component.ReasonCode == "SOURCE_AMBIGUOUS"
		changed = changed || len(component.Operations) > 0
	}
	switch {
	case blocked:
		out.Status = "blocked"
	case ambiguous:
		out.Status = "source_ambiguous"
	case unmetCount > 0 && unmetCount < len(components):
		out.Status = "partially_unmet"
	case unmetCount > 0:
		out.Status = "unmet"
	case changed:
		out.Status = "changed"
	case len(components) == 0:
		out.Status = "not_applicable"
	default:
		out.Status = "unchanged"
	}
	return json.Marshal(out)
}

type partitionConclusion struct {
	Applicable bool `json:"applicable"`
	Blocked    bool `json:"blocked"`
	Unmet      bool `json:"unmet"`
	Changes    bool `json:"changes"`
}
