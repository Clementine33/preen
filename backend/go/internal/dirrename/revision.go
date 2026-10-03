package dirrename

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/onsei/organizer/backend/internal/workset"
)

func (*Task) FreezeExecution(workset.RevisionFacts) (workset.FrozenExecution, []string, error) {
	return workset.FrozenExecution{}, []string{"RENAME_EXECUTION_NOT_IMPLEMENTED"}, nil
}
func (*Task) PrepareUnit(context.Context, workset.UnitRunInput) (workset.PreparedUnit, error) {
	return nil, workset.NewError(
		workset.ErrKindConflict,
		"RENAME_EXECUTION_NOT_IMPLEMENTED",
		"rename execution is not implemented",
		nil,
	)
}

func (*Task) RevisionMembers(in workset.RevisionFacts) ([]workset.RevisionMemberFacts, error) {
	d, err := decodeDraft(in.DraftSnapshot)
	if err != nil {
		return nil, err
	}
	excluded := map[string]bool{}
	for _, m := range d.Members {
		excluded[m.MemberID] = m.Excluded
	}
	payload, _ := json.Marshal(d.Common)
	out := make([]workset.RevisionMemberFacts, 0, len(in.Members))
	for _, m := range in.Members {
		out = append(
			out,
			workset.RevisionMemberFacts{
				MemberID:   m.MemberID,
				FolderPath: m.FolderPath,
				Excluded:   excluded[m.MemberID],
				Payload:    payload,
				Sources:    map[string]string{"template": "common"},
			},
		)
	}
	return out, nil
}
func (*Task) ReviewRevision(in workset.RevisionFacts) (workset.PlanReview, error) {
	if err := validateRevision(in); err != nil {
		return workset.PlanReview{}, err
	}
	review := workset.PlanReview{
		Payload: json.RawMessage(in.Detail.Steps[0].PolicyJSON),
		Tags:    []string{},
		Units:   []workset.UnitReview{},
	}
	for _, c := range in.Detail.Components {
		var o Outcome
		if err := json.Unmarshal([]byte(c.OutcomeJSON), &o); err != nil {
			return review, err
		}
		operations := 0
		if o.Action == "rename" {
			operations = 1
		}
		review.Units = append(
			review.Units,
			workset.UnitReview{Payload: json.RawMessage(c.OutcomeJSON), Operations: operations},
		)
	}
	return review, nil
}
func validateRevision(in workset.RevisionFacts) error {
	if in.Detail == nil || in.Detail.Plan.TaskSchemaVersion != 1 || len(in.Detail.Steps) != 1 ||
		in.Detail.Steps[0].PolicySchemaVersion != 1 {
		return workset.NewError(
			workset.ErrKindInvalidArgument,
			"UNSUPPORTED_SCHEMA_VERSION",
			"unsupported rename revision",
			nil,
		)
	}
	return nil
}
func (t *Task) EvaluateRevision(in workset.RevisionFacts) (workset.RevisionHealth, error) {
	health := workset.RevisionHealth{StaleRoots: []int{}}
	if err := validateRevision(in); err != nil {
		return health, err
	}
	var common Common
	if err := json.Unmarshal([]byte(in.Detail.Steps[0].PolicyJSON), &common); err != nil {
		return health, err
	}
	if err := validateTemplate(common.Template); err != nil {
		return health, err
	}
	paths := make([]string, 0, len(in.Detail.Roots))
	for _, r := range in.Detail.Roots {
		paths = append(paths, r.RootPath)
	}
	ctx := context.Background()
	works, err := t.catalog.RenameWorks(ctx, in.Detail.Plan.RootPath, paths)
	if err != nil {
		return health, err
	}
	directories, readErrors := t.readParentDirectories(ctx, paths)
	outcomes := map[int]Outcome{}
	for _, c := range in.Detail.Components {
		var o Outcome
		if err := json.Unmarshal([]byte(c.OutcomeJSON), &o); err != nil {
			return health, err
		}
		outcomes[c.RootIndex] = o
		health.UnitsBlocked = health.UnitsBlocked || c.Status == "blocked"
	}
	for _, r := range in.Detail.Roots {
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(r.RootPath)))
		directory := directories[parent]
		o := outcomes[r.RootIndex]
		if readErrors[parent] != nil ||
			fingerprint(
				common.Template,
				o,
				works[r.RootPath],
				directory,
				in.Detail.Plan.RootPath,
			) != r.InventoryFingerprint {
			health.StaleRoots = append(health.StaleRoots, r.RootIndex)
		}
	}
	health.InputMoved = len(health.StaleRoots) > 0
	return health, nil
}
