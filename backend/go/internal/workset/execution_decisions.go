package workset

// executionComponents joins only this page's frozen payloads. Tasks own the
// decision format; session results cannot overwrite these plan conclusions.
func (s *serviceImpl) executionComponents(
	planID, kind string,
	units []ExecutionUnit,
) ([]ExecutionComponentView, error) {
	report := initialComponentReport(units)
	task, err := s.requireTask(kind)
	if err != nil {
		return nil, err
	}
	decisions, ok := task.(ExecutionDecisionTask)
	if !ok || len(units) == 0 {
		return report, nil
	}
	indexes := make([]int, 0, len(units))
	for _, unit := range units {
		indexes = append(indexes, unit.Index)
	}
	outcomes, err := s.repo.GetPlanComponentOutcomes(planID, indexes)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load frozen component decisions", err)
	}
	for i := range report {
		outcome, exists := outcomes[report[i].ComponentIndex]
		if !exists {
			return nil, NewError(ErrKindInternal, "INTERNAL", "frozen component outcome is missing", nil)
		}
		kept, err := decisions.KeptDecisions(outcome)
		if err != nil {
			return nil, err
		}
		if len(kept) > 0 && string(kept) != "null" {
			report[i].KeptDecisions = kept
		}
	}
	return report, nil
}
