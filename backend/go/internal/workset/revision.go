package workset

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// validateRevision computes the revision-level validation state from the
// operation's task health:
//   - orphaned worksets: (nil, "unavailable") — there is no live library to
//     validate against;
//   - any root's live input facts moved: (true, "stale");
//   - otherwise: (false, "valid").
func (s *serviceImpl) validateRevision(
	ctx context.Context, operationType string, orphaned bool, detail *PlanDetail,
) (*bool, string) {
	if orphaned {
		return nil, ValidationUnavailable
	}
	stale, err := s.staleInputRoots(ctx, operationType, detail)
	if err != nil || len(stale) > 0 {
		t := true
		return &t, ValidationStale
	}
	f := false
	return &f, ValidationValid
}

// readDetail loads what a summary read needs: the plan frame when the task
// validates inputs from frozen roots alone, otherwise the complete plan.
func (s *serviceImpl) readDetail(operationType, planID string) (*PlanDetail, error) {
	if task, err := s.requireTask(operationType); err == nil {
		if _, ok := task.(InputValidationTask); ok {
			return s.repo.GetPlanFrame(planID)
		}
	}
	return s.repo.GetPlanDetail(planID)
}

// staleInputRoots reports the roots whose live inputs moved. Tasks that declare
// InputValidationTask answer from the roots alone; the rest evaluate the
// complete plan.
func (s *serviceImpl) staleInputRoots(
	ctx context.Context, operationType string, detail *PlanDetail,
) ([]int, error) {
	task, err := s.requireTask(operationType)
	if err != nil {
		return nil, err
	}
	if v, ok := task.(InputValidationTask); ok {
		return v.StaleInputRoots(ctx, detail.Roots)
	}
	health, err := s.revisionHealth(ctx, operationType, detail)
	return health.StaleRoots, err
}

// loadCurrentRevision builds the compact immutable conclusion of an operation's
// current revision and returns the already-loaded roots for coverage.
func (s *serviceImpl) loadCurrentRevision(
	ctx context.Context, op *Operation,
) (*RevisionSummary, []PlanRootRecord, error) {
	rev, err := s.repo.GetOperationRevision(op.WorksetID, op.OperationType, op.CurrentRevisionID)
	if err != nil {
		return nil, nil, NewError(ErrKindInternal, "INTERNAL", "failed to load current revision", err)
	}
	detail, err := s.readDetail(op.OperationType, op.CurrentRevisionID)
	if err != nil {
		return nil, nil, NewError(ErrKindInternal, "INTERNAL", "failed to load revision plan", err)
	}
	w, err := s.repo.GetWorkset(op.WorksetID)
	if err != nil {
		return nil, nil, NewError(ErrKindInternal, "INTERNAL", "failed to load workset", err)
	}
	stale, validation := s.validateRevision(ctx, op.OperationType, w.LibraryID == "", detail)
	return &RevisionSummary{
		PlanID:          detail.Plan.PlanID,
		RevisionIndex:   rev.RevisionIndex,
		CreatedAt:       detail.Plan.CreatedAt,
		Status:          detail.Plan.Status,
		SummaryReason:   detail.Plan.SummaryReason,
		Counts:          detail.Plan.Counts,
		ValidationState: validation,
		Stale:           stale,
	}, detail.Roots, nil
}

// revisionReview asks the operation's task for the reviewable view of one
// persisted revision.
func (s *serviceImpl) revisionReview(
	operationType string, detail *PlanDetail,
) (PlanReview, error) {
	task, err := s.requireTask(operationType)
	if err != nil {
		return PlanReview{}, err
	}
	return task.ReviewRevision(RevisionFacts{Detail: detail})
}

// planSummaryOf parses the generic plan-summary facts out of a persisted
// revision.
func planSummaryOf(detail *PlanDetail) PlanSummary {
	if len(detail.Steps) == 0 {
		return PlanSummary{}
	}
	var summary PlanSummary
	_ = json.Unmarshal([]byte(detail.Steps[0].StepSummaryJSON), &summary)
	return summary
}

// stampRevisionSummary derives the revision's counts and summary reason from
// the complete in-memory frozen snapshot, before it is staged.
func (s *serviceImpl) stampRevisionSummary(operationType string, p *OperationRevisionPersist) error {
	task, err := s.requireTask(operationType)
	if err != nil {
		return err
	}
	counts, reason, err := summarizeRevision(task, &PlanDetail{
		Plan:       Plan{TaskSchemaVersion: p.TaskSchemaVersion},
		Steps:      p.Steps,
		Roots:      p.Roots,
		Components: p.Components,
	})
	if err != nil {
		return err
	}
	p.Counts, p.SummaryReason = counts, reason
	return nil
}

// stampMemberConclusions derives the frozen part of every record member's
// conclusion from the same complete in-memory snapshot, before staging.
func (s *serviceImpl) stampMemberConclusions(
	operationType string, members []*WorksetMember, p *OperationRevisionPersist,
) error {
	task, err := s.requireTask(operationType)
	if err != nil {
		return err
	}
	excluded := excludedSet(p.ExcludedScope)
	revisionMembers := make([]RevisionMember, 0, len(members))
	for _, member := range members {
		revisionMembers = append(revisionMembers, RevisionMember{
			MemberID: member.MemberID, Excluded: excluded[member.MemberID],
		})
	}
	p.Conclusions, err = freezeMemberConclusions(revisionMembers, p.Roots, p.Components, task)
	return err
}

// summarizeRevision is the one rule behind a revision's counts and summary
// reason; it depends only on the frozen plan and the task's review of it.
func summarizeRevision(task Task, detail *PlanDetail) (RevisionCounts, string, error) {
	review, err := task.ReviewRevision(RevisionFacts{Detail: detail})
	if err != nil {
		return RevisionCounts{}, "", err
	}
	return revisionCounts(detail, review), planSummaryOf(detail).SummaryReason, nil
}

// revisionCounts derives the four independent plan facts from the frozen
// snapshot. Changed, unmet, blocked and unchanged are separate facts that may
// overlap; no exclusive status label is used to derive them (ADR 0001).
func revisionCounts(detail *PlanDetail, review PlanReview) RevisionCounts {
	summary := planSummaryOf(detail)
	withOperations := map[string]bool{}
	blockedComponents := 0
	blockedRoots := map[int]bool{}
	for i, c := range detail.Components {
		if c.Status == "blocked" {
			blockedComponents++
			blockedRoots[c.RootIndex] = true
			continue
		}
		if i < len(review.Units) && review.Units[i].Operations > 0 {
			withOperations[c.ComponentID] = true
		}
	}
	missingRoots := 0
	for _, r := range detail.Roots {
		if r.RootStatus == "missing" && !blockedRoots[r.RootIndex] {
			// A task may persist a blocked unit for a missing root; count it once.
			missingRoots++
		}
	}
	componentCount := summary.ComponentCount
	if componentCount == 0 {
		componentCount = len(detail.Components)
	}
	changed := len(withOperations)
	return RevisionCounts{
		Members:      len(detail.Roots),
		Changed:      changed,
		UnmetTargets: summary.UnmetTargets,
		Blocked:      blockedComponents + missingRoots,
		Unchanged:    componentCount - changed - blockedComponents,
	}
}

// GetRevision returns the current plan summary and per-member conclusions.
// Frozen settings and complete component outcomes are read through GetMemberReview.
func (s *serviceImpl) GetRevision(
	ctx context.Context,
	worksetID, operationType, planID string,
) (*RevisionView, error) {
	return s.readRevision(ctx, worksetID, operationType, planID, "", 0)
}

// GetRevisionCategory is GetRevision with Members narrowed to one category.
func (s *serviceImpl) GetRevisionCategory(
	ctx context.Context,
	worksetID, operationType, planID, category string,
	limit int,
) (*RevisionView, error) {
	return s.readRevision(ctx, worksetID, operationType, planID, category, limit)
}

func (s *serviceImpl) readRevision(
	ctx context.Context,
	worksetID, operationType, planID, category string,
	limit int,
) (*RevisionView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := s.loadOperation(worksetID, operationType); err != nil {
		return nil, err
	}
	rev, err := s.repo.GetOperationRevision(worksetID, operationType, planID)
	if err != nil {
		if errors.Is(err, ErrRevisionNotFound) {
			return nil, NewError(ErrKindNotFound, "REVISION_NOT_FOUND", "revision not found", nil)
		}
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load revision", err)
	}
	detail, err := s.readDetail(operationType, planID)
	if err != nil {
		return nil, NewError(ErrKindNotFound, "REVISION_NOT_FOUND", "revision not found", nil)
	}
	w, err := s.repo.GetWorkset(worksetID)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load workset", err)
	}
	staleRoots := map[int]bool{}
	if w.LibraryID != "" {
		stale, healthErr := s.staleInputRoots(ctx, operationType, detail)
		if healthErr != nil {
			return nil, NewError(ErrKindInternal, "INTERNAL", "failed to validate revision inputs", healthErr)
		}
		for _, idx := range stale {
			staleRoots[idx] = true
		}
	}

	// The response carries only the plan-level payload and tags, which the
	// steps own; unit outcomes are read through GetMemberReview.
	frame := *detail
	frame.Components = nil
	review, reviewErr := s.revisionReview(operationType, &frame)
	if reviewErr != nil {
		return nil, reviewErr
	}
	out := &RevisionView{
		PlanID:        planID,
		RevisionIndex: rev.RevisionIndex,
		CreatedAt:     rev.CreatedAt,
		Counts:        detail.Plan.Counts,
	}
	members, mErr := s.repo.ListWorksetMembers(worksetID)
	if mErr != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load members", mErr)
	}
	stored, sErr := s.repo.ListMemberConclusions(planID, nil)
	if sErr != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load member conclusions", sErr)
	}
	concluded := make([]RevisionMember, 0, len(members))
	for _, member := range members {
		concluded = append(concluded, RevisionMember{MemberID: member.MemberID, FolderPath: member.FolderPath})
	}
	if err := concludeRevisionMembers(concluded, stored, detail.Roots, w.LibraryID == "", staleRoots); err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to conclude revision members", err)
	}
	out.Members = make([]MemberConclusion, 0, len(concluded))
	out.CategoryCounts = map[string]int{}
	for _, member := range concluded {
		if c := member.Conclusion.Category; c != "" {
			out.CategoryCounts[c]++
		}
		if category != "" && member.Conclusion.Category != category {
			continue
		}
		out.CategoryTotal++
		if category == "" || len(out.Members) < limit {
			out.Members = append(out.Members, member.Conclusion)
		}
	}
	out.Category = category
	review.Units = nil
	out.Plan = revisionPlan(detail, review)
	executed, execErr := s.repo.GetExecutionForRevision(planID)
	if execErr != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load revision execution", execErr)
	}
	if executed != nil {
		out.Execution = executionRefOf(executed)
	}
	return out, nil
}

// revisionMemberFacts asks the operation's task to resolve the frozen draft
// snapshot into per-member effective configs.
func (s *serviceImpl) revisionMemberFacts(
	operationType string, draftSnapshot string, members []*WorksetMember,
) ([]RevisionMemberFacts, error) {
	task, err := s.requireTask(operationType)
	if err != nil {
		return nil, err
	}
	return task.RevisionMembers(RevisionFacts{DraftSnapshot: []byte(draftSnapshot), Members: members})
}

// revisionMembers joins the task's resolved member configs with the workset's
// member names and the revision's frozen exclusion scope. An unreadable
// snapshot yields no members rather than failing the whole historical revision
// read.
func revisionMembers(
	facts []RevisionMemberFacts, members []*WorksetMember, excludedScope string,
) []RevisionMember {
	if len(facts) == 0 {
		return nil
	}
	names := map[string]string{}
	for _, m := range members {
		names[m.MemberID] = m.FolderName
	}
	excluded := excludedSet(excludedScope)
	out := make([]RevisionMember, 0, len(facts))
	for _, f := range facts {
		// Exclusion is authoritative in the frozen scope; the snapshot's own
		// flags are the same fact recorded twice.
		out = append(out, RevisionMember{
			MemberID:   f.MemberID,
			FolderPath: f.FolderPath,
			MemberName: names[f.MemberID],
			Excluded:   f.Excluded || excluded[f.MemberID],
			Payload:    f.Payload,
			Sources:    f.Sources,
		})
	}
	return out
}

func excludedSet(scope string) map[string]bool {
	out := map[string]bool{}
	if scope == "" {
		return out
	}
	for id := range strings.SplitSeq(scope, "\x00") {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

// planView rebuilds one revision's reviewable plan from its persisted records
// — never from live policy/classifier state. A revision carries exactly one
// plan, so the persisted single step flattens into it; the payload decode
// belongs to the operation's task.
func (s *serviceImpl) planView(operationType string, detail *PlanDetail) RevisionPlan {
	review, err := s.revisionReview(operationType, detail)
	if err != nil {
		review = PlanReview{}
	}
	return revisionPlan(detail, review)
}

func revisionPlan(detail *PlanDetail, review PlanReview) RevisionPlan {
	out := RevisionPlan{
		PlanID:            detail.Plan.PlanID,
		SnapshotToken:     detail.Plan.SnapshotToken,
		RootPath:          detail.Plan.RootPath,
		TaskKind:          detail.Plan.TaskKind,
		TaskSchemaVersion: detail.Plan.TaskSchemaVersion,
		Summary:           planSummaryOf(detail),
	}
	if len(detail.Steps) > 0 {
		st := detail.Steps[0]
		out.Status = st.Status
		out.PolicyHash = st.PolicyHash
		out.StepSummary = json.RawMessage(st.StepSummaryJSON)
	}
	out.Payload = review.Payload
	out.ClassifierTags = review.Tags
	out.ClassifierHash = review.TagHash
	for _, u := range review.Units {
		out.Units = append(out.Units, u.Payload)
	}
	return out
}
