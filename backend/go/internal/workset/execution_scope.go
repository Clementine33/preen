package workset

import "sort"

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

func errMemberNotInRecord(id string) error {
	return NewError(ErrKindInvalidArgument, "MEMBER_NOT_IN_RECORD", "not a member of this record: "+id, nil)
}

// expandExecutionScope turns the request's scope into the sorted member ids the
// session runs, so the same set of members is the same request however it was
// written; nil means the whole revision. Every named id must be a member of
// the record, and a scope that excludes every member is refused rather than
// read as the whole revision.
func (s *serviceImpl) expandExecutionScope(worksetID string, req StartExecutionRequest) ([]string, error) {
	if len(req.MemberIDs) == 0 && len(req.AllExceptMemberIDs) == 0 {
		return nil, nil
	}
	if len(req.MemberIDs) > 0 && len(req.AllExceptMemberIDs) > 0 {
		return nil, NewError(
			ErrKindInvalidArgument, "INVALID_SCOPE", "member_ids and all_except_member_ids do not combine", nil,
		)
	}
	members, err := s.repo.ListWorksetMembers(worksetID)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load members", err)
	}
	known := make(map[string]bool, len(members))
	for _, m := range members {
		known[m.MemberID] = true
	}
	include := len(req.MemberIDs) > 0
	named := req.AllExceptMemberIDs
	if include {
		named = req.MemberIDs
	}
	picked := make(map[string]bool, len(named))
	for _, id := range named {
		if !known[id] {
			return nil, errMemberNotInRecord(id)
		}
		picked[id] = true
	}
	out := make([]string, 0, len(known))
	for _, m := range members {
		if picked[m.MemberID] == include {
			out = append(out, m.MemberID)
		}
	}
	if len(out) == 0 {
		return nil, NewError(ErrKindInvalidArgument, "INVALID_SCOPE", "the scope selects no member", nil)
	}
	sort.Strings(out)
	return out, nil
}
