package conversion

import (
	"encoding/json"

	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// KeptDecisions returns the frozen file conclusions an execution leaves alone,
// including unmet targets, regardless of the component's execution status.
func (*Task) KeptDecisions(raw json.RawMessage) (json.RawMessage, error) {
	var outcome reconcile.ComponentOutcome
	if err := json.Unmarshal(raw, &outcome); err != nil {
		return nil, workset.NewError(workset.ErrKindInternal, "INTERNAL", "stored component outcome is invalid", err)
	}
	kept := []reconcile.FileDecision{}
	for _, variant := range outcome.Variants {
		for _, decision := range variant.Decisions {
			if decision.Resolution == "keep" {
				kept = append(kept, decision)
			}
		}
	}
	return json.Marshal(kept)
}
