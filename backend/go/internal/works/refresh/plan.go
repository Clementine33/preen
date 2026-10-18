package refresh

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Clementine33/preen/backend/internal/librarypath"
	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/workset"
)

type Outcome struct {
	MemberID   string               `json:"member_id"`
	Target     coverdelivery.Target `json:"target"`
	Action     string               `json:"action"`
	ReasonCode string               `json:"reason_code,omitempty"`
}

const reasonAlreadyComplete = "ALREADY_COMPLETE"

// completeWorks reports the bound works fill_missing would fetch nothing for,
// whatever tag language their stored data was fetched in.
func (t *Task) completeWorks(
	ctx context.Context,
	mode Mode,
	bindings map[string]coverdelivery.Target,
) (map[string]bool, error) {
	out := map[string]bool{}
	if mode != FillMissing {
		return out, nil
	}
	ids := []works.WorkCode{}
	for _, b := range bindings {
		if b.CopyID != "" && !out[b.WorkCode] {
			out[b.WorkCode] = false
			ids = append(ids, works.WorkCode(b.WorkCode))
		}
	}
	states, err := t.catalog.MetadataStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		missing := false
		for _, need := range wanted(FillMissing, states[id]) {
			missing = missing || need
		}
		out[string(id)] = !missing
	}
	return out, nil
}

func (t *Task) facts(folder string, target coverdelivery.Target) (coverdelivery.Target, string, error) {
	err := t.checkDirectory(folder)
	target.Identity = ""
	b, _ := json.Marshal(folder)
	return target, workset.CanonicalJSONHash(b), err
}
func (t *Task) PlanSession(ctx context.Context, in workset.PlanSessionInput) (*workset.PlanSnapshot, error) {
	if err := t.ValidateSessionInput(in.RawDraft, in.Members); err != nil {
		return nil, err
	}
	if len(in.Members) == 0 || strings.TrimSpace(in.LibraryRoot) == "" {
		return nil, invalid("library root and members are required")
	}
	d, _ := decode(in.RawDraft)
	root := librarypath.CleanRootPath(in.LibraryRoot)
	excluded := map[string]bool{}
	excludedIDs := []string{}
	for _, m := range d.Members {
		if m.Excluded {
			excluded[m.MemberID] = true
			excludedIDs = append(excludedIDs, m.MemberID)
		}
	}
	paths := []string{}
	for _, m := range in.Members {
		rel, ok := librarypath.RelPath(m.RelPath)
		if !ok || librarypath.RootPathKey(librarypath.JoinRel(root, rel)) != librarypath.RootPathKey(m.FolderPath) {
			return nil, invalid("members must be beneath the saved library root")
		}
		if !excluded[m.MemberID] {
			paths = append(paths, m.FolderPath)
		}
	}
	bindings, err := t.catalog.CoverTargets(ctx, root, paths)
	if err != nil {
		return nil, err
	}
	complete, err := t.completeWorks(ctx, d.Common.Mode, bindings)
	if err != nil {
		return nil, err
	}
	snap := &workset.PlanSnapshot{
		StepType:             "refresh_metadata",
		RootPath:             root,
		PayloadSchemaVersion: 1,
		Status:               "ready",
		ExcludedScope:        strings.Join(excludedIDs, "\x00"),
	}
	snap.Payload, _ = json.Marshal(d.Common)
	snap.PayloadHash = workset.CanonicalJSONHash(snap.Payload)
	var summary struct {
		workset.PlanSummary

		WorkCount int `json:"work_count"`
	}
	summary.SummaryReason = "ok"
	codes := map[string]bool{}
	for _, m := range in.Members {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if excluded[m.MemberID] {
			continue
		}
		index := len(snap.Roots)
		target := bindings[m.FolderPath]
		unit, r := t.planMember(m, target, index, complete[target.WorkCode], d.Common.DownloadCover)
		if target.CopyID != "" && unit.ReasonCode != reasonAlreadyComplete {
			codes[target.WorkCode] = true
			summary.OperationCount++
		}
		if unit.Status == "blocked" {
			summary.ErrorCount++
			summary.BlockedCount++
		}
		snap.Units = append(snap.Units, unit)
		snap.Roots = append(snap.Roots, r)
		if in.Progress != nil {
			in.Progress(
				workset.PlanProgress{CompletedRoots: index + 1, TotalRoots: len(paths), CurrentRoot: m.FolderPath},
			)
		}
	}
	summary.ComponentCount = len(snap.Units)
	summary.WorkCount = len(codes)
	if summary.BlockedCount > 0 {
		summary.SummaryReason = "blocked"
	}
	snap.Summary, _ = json.Marshal(summary)
	return snap, nil
}

func (t *Task) planMember(
	m *workset.WorksetMember,
	target coverdelivery.Target,
	index int,
	complete, downloadCover bool,
) (workset.PlannedUnit, workset.PlanRootFacts) {
	target, hash, err := t.facts(m.FolderPath, target)
	outcome := Outcome{MemberID: m.MemberID, Target: target, Action: "refresh"}
	root := workset.PlanRootFacts{
		Index:                index,
		Path:                 m.FolderPath,
		Identity:             target.Identity,
		InventoryFingerprint: hash,
		Count:                1,
		Status:               "ok",
	}
	status := "ok"
	if target.CopyID == "" {
		outcome.Action = "skip"
		outcome.ReasonCode = "NO_WORK_BINDING"
	}
	if complete && target.CopyID != "" {
		// Nothing to fetch, but a copy folder may still lack the cover.
		outcome.Action = "skip"
		if downloadCover {
			outcome.Action = "cover"
		}
		outcome.ReasonCode = reasonAlreadyComplete
	}
	if err != nil {
		status = "blocked"
		root.Status = "missing"
		root.Count = 0
		root.ErrorCode = "SOURCE_MISSING"
		root.ErrorMessage = err.Error()
		outcome.Action = "skip"
		outcome.ReasonCode = "SOURCE_MISSING"
	}
	payload, _ := json.Marshal(outcome)
	return workset.PlannedUnit{
		Index:      index,
		RootIndex:  index,
		ID:         m.MemberID,
		Status:     status,
		ReasonCode: outcome.ReasonCode,
		Payload:    payload,
	}, root
}
