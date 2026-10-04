package refresh

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"slices"

	"github.com/onsei/organizer/backend/internal/workset"
)

func validate(in workset.RevisionFacts) error {
	if in.Detail == nil || in.Detail.Plan.TaskSchemaVersion != 1 || len(in.Detail.Steps) != 1 ||
		in.Detail.Steps[0].PolicySchemaVersion != 1 {
		return workset.NewError(
			workset.ErrKindInvalidArgument,
			"UNSUPPORTED_SCHEMA_VERSION",
			"unsupported metadata revision",
			nil,
		)
	}
	var c Common
	if err := json.Unmarshal([]byte(in.Detail.Steps[0].PolicyJSON), &c); err != nil {
		return err
	}
	return validateCommon(c)
}
func (t *Task) EvaluateRevision(in workset.RevisionFacts) (workset.RevisionHealth, error) {
	h := workset.RevisionHealth{StaleRoots: []int{}}
	if err := validate(in); err != nil {
		return h, err
	}
	for _, r := range in.Detail.Roots {
		if in.InputRoots != nil && !slices.Contains(in.InputRoots, r.RootPath) {
			continue
		}
		err := t.checkDirectory(r.RootPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return h, workset.NewError(
				workset.ErrKindInternal,
				"SCAN_FAILED",
				"cannot check frozen work directory: "+r.RootPath,
				err,
			)
		}
		if err != nil || r.RootStatus == "missing" {
			h.StaleRoots = append(h.StaleRoots, r.RootIndex)
		}
	}
	for _, c := range in.Detail.Components {
		h.UnitsBlocked = h.UnitsBlocked || c.Status == "blocked"
	}
	h.InputMoved = len(h.StaleRoots) > 0
	return h, nil
}
func (*Task) FreezeExecution(in workset.RevisionFacts) (workset.FrozenExecution, []string, error) {
	out := workset.FrozenExecution{}
	if err := validate(in); err != nil {
		return out, nil, err
	}
	d, err := decode(in.DraftSnapshot)
	if err != nil {
		return out, nil, err
	}
	out.Options, _ = json.Marshal(d.Common)
	roots := map[int]string{}
	for _, r := range in.Detail.Roots {
		roots[r.RootIndex] = r.RootPath
	}
	for _, c := range in.Detail.Components {
		var o Outcome
		if err := json.Unmarshal([]byte(c.OutcomeJSON), &o); err != nil {
			return out, nil, err
		}
		ops := 0
		if o.Action == "refresh" {
			ops = 1
		}
		out.Units = append(
			out.Units,
			workset.ExecutionUnit{
				Index:      c.ComponentIndex,
				RootIndex:  c.RootIndex,
				ID:         c.ComponentID,
				RootPath:   roots[c.RootIndex],
				Operations: ops,
				Payload:    json.RawMessage(c.OutcomeJSON),
			},
		)
		out.TotalOperations += ops
	}
	return out, nil, nil
}
func (*Task) PrepareUnit(context.Context, workset.UnitRunInput) (workset.PreparedUnit, error) {
	return nil, workset.NewError(
		workset.ErrKindInternal,
		"BATCH_EXECUTOR_REQUIRED",
		"metadata task requires batch execution",
		nil,
	)
}
func (*Task) RevisionMembers(in workset.RevisionFacts) ([]workset.RevisionMemberFacts, error) {
	d, err := decode(in.DraftSnapshot)
	if err != nil {
		return nil, err
	}
	excluded := map[string]bool{}
	for _, m := range d.Members {
		excluded[m.MemberID] = m.Excluded
	}
	payload, _ := json.Marshal(d.Common)
	out := []workset.RevisionMemberFacts{}
	for _, m := range in.Members {
		out = append(
			out,
			workset.RevisionMemberFacts{
				MemberID:   m.MemberID,
				FolderPath: m.FolderPath,
				Excluded:   excluded[m.MemberID],
				Payload:    payload,
				Sources:    map[string]string{"mode": "common", "locale": "common", "download_cover": "common"},
			},
		)
	}
	return out, nil
}
func (*Task) ReviewRevision(in workset.RevisionFacts) (workset.PlanReview, error) {
	if err := validate(in); err != nil {
		return workset.PlanReview{}, err
	}
	out := workset.PlanReview{
		Payload: json.RawMessage(in.Detail.Steps[0].PolicyJSON),
		Tags:    []string{},
		Units:   []workset.UnitReview{},
	}
	for _, c := range in.Detail.Components {
		var o Outcome
		if err := json.Unmarshal([]byte(c.OutcomeJSON), &o); err != nil {
			return out, err
		}
		ops := 0
		if o.Action == "refresh" {
			ops = 1
		}
		out.Units = append(out.Units, workset.UnitReview{Payload: json.RawMessage(c.OutcomeJSON), Operations: ops})
	}
	return out, nil
}
