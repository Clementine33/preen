package dirrename

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/Clementine33/preen/backend/internal/workset"
)

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
func (t *Task) EvaluateRevision(ctx context.Context, in workset.RevisionFacts) (workset.RevisionHealth, error) {
	health := workset.RevisionHealth{StaleRoots: []int{}}
	if err := ctx.Err(); err != nil {
		return health, err
	}
	if err := validateRevision(in); err != nil {
		return health, err
	}
	var common Common
	if err := json.Unmarshal([]byte(in.Detail.Steps[0].PolicyJSON), &common); err != nil {
		return health, err
	}
	if err := validateCommon(common); err != nil {
		return health, err
	}
	paths := make([]string, 0, len(in.Detail.Roots))
	for _, r := range in.Detail.Roots {
		if in.InputRoots == nil || slices.Contains(in.InputRoots, r.RootPath) {
			paths = append(paths, r.RootPath)
		}
	}
	works, err := t.catalog.RenameWorks(ctx, in.Detail.Plan.RootPath, paths)
	if err != nil {
		return health, err
	}
	directories, readErrors := t.readParentDirectories(ctx, paths)
	for _, err := range readErrors {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return health, err
		}
	}
	outcomes := map[int]Outcome{}
	for _, c := range in.Detail.Components {
		var o Outcome
		if err := json.Unmarshal([]byte(c.OutcomeJSON), &o); err != nil {
			return health, err
		}
		outcomes[c.RootIndex] = o
	}
	// A blocked folder is skipped by the run, never a reason to refuse it. A
	// followed hand rename reads the work bound to the folder's new location
	// when it already has one.
	var lookups []string
	for _, r := range in.Detail.Roots {
		if o := outcomes[r.RootIndex]; o.ObservedName != "" && slices.Contains(paths, r.RootPath) {
			lookups = append(
				lookups,
				filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(r.RootPath)), o.ObservedName)),
			)
		}
	}
	if len(lookups) > 0 {
		found, err := t.catalog.RenameWorks(ctx, in.Detail.Plan.RootPath, lookups)
		if err != nil {
			return health, err
		}
		for _, r := range in.Detail.Roots {
			o := outcomes[r.RootIndex]
			if o.ObservedName == "" {
				continue
			}
			observed := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(r.RootPath)), o.ObservedName))
			if w, ok := found[observed]; ok {
				works[r.RootPath] = w
			}
		}
	}
	for _, r := range in.Detail.Roots {
		if !slices.Contains(paths, r.RootPath) {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(r.RootPath)))
		directory := directories[parent]
		o := outcomes[r.RootIndex]
		live := planName(common, &workset.WorksetMember{
			MemberID: o.MemberID, FolderPath: r.RootPath,
		}, works[r.RootPath], directory, parent, naming{Custom: o.CustomName, Observed: o.ObservedName})
		// Batch collisions are frozen globally; a per-root read cannot recreate
		// that reason. Comparing proposals also supports older mtime fingerprints.
		if o.ReasonCode == "TARGET_COLLISION" && live.ReasonCode == "" {
			live.Action, live.ReasonCode = o.Action, o.ReasonCode
		}
		if readErrors[parent] != nil || live != o {
			health.StaleRoots = append(health.StaleRoots, r.RootIndex)
		}
	}
	health.InputMoved = len(health.StaleRoots) > 0
	return health, nil
}
