package metadatatask

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/onsei/organizer/backend/internal/covers"
	"github.com/onsei/organizer/backend/internal/pathnorm"
	"github.com/onsei/organizer/backend/internal/workset"
)

type Outcome struct {
	MemberID   string        `json:"member_id"`
	Target     covers.Target `json:"target"`
	Action     string        `json:"action"`
	ReasonCode string        `json:"reason_code,omitempty"`
}

func (t *Task) facts(folder string, target covers.Target) (covers.Target, string, error) {
	id, err := t.identity(folder)
	target.Identity = id
	b, _ := json.Marshal(target)
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
	root := pathnorm.CleanRootPath(in.LibraryRoot)
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
		rel, ok := pathnorm.RelPath(m.RelPath)
		if !ok || pathnorm.RootPathKey(pathnorm.JoinRel(root, rel)) != pathnorm.RootPathKey(m.FolderPath) {
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
	snap := &workset.PlanSnapshot{
		StepType:             "refresh_metadata",
		RootPath:             root,
		PayloadSchemaVersion: 1,
		Status:               "ready",
		ExcludedScope:        strings.Join(excludedIDs, "\x00"),
	}
	snap.Payload, _ = json.Marshal(d.Common)
	snap.PayloadHash = workset.CanonicalJSONHash(snap.Payload)
	summary := struct {
		workset.PlanSummary

		WorkCount int `json:"work_count"`
	}{SummaryReason: "ok"}
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
		unit, r := t.planMember(m, target, index)
		if target.CopyID != "" {
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
	target covers.Target,
	index int,
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
