package workset

import (
	"context"
	"errors"
)

// GetMemberReview resolves frozen settings and validates only this member's
// persisted inputs. Other members' component outcomes are never loaded.
func (s *serviceImpl) GetMemberReview(
	ctx context.Context,
	worksetID, operationType, planID, memberID string,
) (*MemberReview, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	op, err := s.loadOperation(worksetID, operationType)
	if err != nil {
		return nil, err
	}
	if op.CurrentRevisionID != planID {
		return nil, NewError(ErrKindNotFound, "REVISION_NOT_FOUND", "revision not found", nil)
	}
	rev, err := s.repo.GetOperationRevision(worksetID, operationType, planID)
	if err != nil {
		if errors.Is(err, ErrRevisionNotFound) {
			return nil, NewError(ErrKindNotFound, "REVISION_NOT_FOUND", "revision not found", nil)
		}
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load revision", err)
	}
	members, err := s.repo.ListWorksetMembers(worksetID)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load members", err)
	}
	var selected *WorksetMember
	for _, member := range members {
		if member.MemberID == memberID {
			selected = member
			break
		}
	}
	if selected == nil {
		return nil, NewError(ErrKindNotFound, "MEMBER_NOT_FOUND", "member not found", nil)
	}
	facts, err := s.revisionMemberFacts(operationType, rev.DraftSnapshot, members)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to resolve frozen settings", err)
	}
	member := RevisionMember{
		MemberID:   memberID,
		FolderPath: selected.FolderPath,
		MemberName: selected.FolderName,
		Excluded:   excludedSet(rev.ExcludedScope)[memberID],
	}
	for _, frozen := range revisionMembers(facts, members, rev.ExcludedScope) {
		if frozen.MemberID == memberID {
			member = frozen
			break
		}
	}
	ownerID := member.MemberID
	if member.Excluded {
		ownerID = ""
	}
	detail, err := s.repo.GetMemberPlanDetail(planID, ownerID)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			return nil, NewError(ErrKindNotFound, "REVISION_NOT_FOUND", "revision not found", nil)
		}
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load member plan", err)
	}
	w, err := s.loadWorkset(worksetID)
	if err != nil {
		return nil, err
	}
	staleRoots := map[int]bool{}
	if w.LibraryID != "" && len(detail.Roots) > 0 {
		health, healthErr := s.revisionHealth(ctx, operationType, detail)
		if healthErr != nil {
			return nil, NewError(ErrKindInternal, "INTERNAL", "failed to validate member inputs", healthErr)
		}
		for _, index := range health.StaleRoots {
			staleRoots[index] = true
		}
	}
	stored, err := s.repo.ListMemberConclusions(planID, &memberID)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load member conclusion", err)
	}
	owned := []RevisionMember{member}
	if err := concludeRevisionMembers(owned, stored, detail.Roots, w.LibraryID == "", staleRoots); err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to conclude member", err)
	}
	review := &MemberReview{PlanID: planID, Member: owned[0], Plan: s.planView(operationType, detail)}
	for _, root := range detail.Roots {
		review.Roots = append(
			review.Roots,
			RootValidation{
				RootIndex:            root.RootIndex,
				RootPath:             root.RootPath,
				RootStatus:           root.RootStatus,
				RootErrorCode:        root.RootErrorCode,
				RootErrorMessage:     root.RootErrorMessage,
				Stale:                staleRoots[root.RootIndex],
				InventoryFingerprint: root.InventoryFingerprint,
				EntryCount:           root.EntryCount,
			},
		)
	}
	return review, nil
}
