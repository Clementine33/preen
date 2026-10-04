package workset

// executionInputRoots derives freshness scope from the same component ownership
// used by frozen execution units. Revision detail deliberately passes no scope.
func (s *serviceImpl) executionInputRoots(worksetID string, detail *PlanDetail, selection []string) ([]string, error) {
	roots := make(map[int]string, len(detail.Roots))
	for _, r := range detail.Roots {
		roots[r.RootIndex] = r.RootPath
	}
	units := make([]ExecutionUnit, 0, len(detail.Components))
	for _, c := range detail.Components {
		units = append(units, ExecutionUnit{RootPath: roots[c.RootIndex]})
	}
	scoped, err := s.scopeExecution(worksetID, FrozenExecution{Units: units}, selection)
	if err != nil {
		return nil, err
	}
	paths := unitRoots(scoped.Units)
	if paths == nil {
		paths = []string{}
	}
	return paths, nil
}
