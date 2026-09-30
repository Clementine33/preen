package rename

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/onsei/organizer/backend/internal/pathnorm"
	"github.com/onsei/organizer/backend/internal/workset"
)

// Outcome freezes the original and cleaned names and the member's block reason.
type Outcome struct {
	MemberID   string `json:"member_id"`
	SourceName string `json:"source_name"`
	WorkCode   string `json:"work_code"`
	RawName    string `json:"raw_name"`
	TargetName string `json:"target_name"`
	Action     string `json:"action"`
	ReasonCode string `json:"reason_code,omitempty"`
}

func (t *Task) PlanSession(ctx context.Context, in workset.PlanSessionInput) (*workset.PlanSnapshot, error) {
	if err := t.ValidateSessionInput(in.RawDraft, in.Members); err != nil {
		return nil, err
	}
	if len(in.Members) == 0 {
		return nil, invalid("no members")
	}
	d, _ := decodeDraft(in.RawDraft)
	root := filepath.ToSlash(filepath.Dir(filepath.FromSlash(in.Members[0].FolderPath)))
	paths, excluded, excludedIDs, err := planningScope(root, d, in.Members)
	if err != nil {
		return nil, err
	}

	works, err := t.catalog.RenameWorks(ctx, root, paths)
	if err != nil {
		return nil, err
	}
	directory, err := t.readDirectory(ctx, root)
	if err != nil {
		return nil, err
	}
	snapshot := &workset.PlanSnapshot{
		RootPath:             root,
		PayloadSchemaVersion: 1,
		Status:               "ready",
		ExcludedScope:        strings.Join(excludedIDs, "\x00"),
	}
	snapshot.Payload, _ = json.Marshal(d.Common)
	snapshot.PayloadHash = workset.CanonicalJSONHash(snapshot.Payload)
	summary := workset.PlanSummary{SummaryReason: "ok"}
	outcomes := map[string]Outcome{}
	targets := map[string]int{}
	for _, m := range in.Members {
		if excluded[m.MemberID] {
			continue
		}
		o := planName(d.Common.Template, m, works[m.FolderPath], directory, root)
		outcomes[m.MemberID] = o
		if o.ReasonCode == "" {
			targets[pathnorm.RootPathKey(filepath.ToSlash(filepath.Join(root, o.TargetName)))]++
		}
	}
	for _, m := range in.Members {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if excluded[m.MemberID] {
			continue
		}
		outcome := outcomes[m.MemberID]
		if outcome.ReasonCode == "" &&
			targets[pathnorm.RootPathKey(filepath.ToSlash(filepath.Join(root, outcome.TargetName)))] > 1 {
			outcome.Action = "blocked"
			outcome.ReasonCode = "TARGET_COLLISION"
		}
		index := len(snapshot.Units)
		status := "ok"
		if outcome.ReasonCode != "" {
			status = "blocked"
			summary.BlockedCount++
		}
		if outcome.Action == "rename" {
			summary.OperationCount++
		}
		payload, _ := json.Marshal(outcome)
		snapshot.Units = append(
			snapshot.Units,
			workset.PlannedUnit{
				Index:      index,
				RootIndex:  index,
				ID:         m.MemberID,
				Status:     status,
				ReasonCode: outcome.ReasonCode,
				Payload:    payload,
			},
		)
		facts := freezeRoot(index, m, d.Common.Template, outcome, works[m.FolderPath], directory, root)
		snapshot.Roots = append(snapshot.Roots, facts)
		if facts.Status == "missing" {
			summary.ErrorCount++
		}

		if in.Progress != nil {
			in.Progress(
				workset.PlanProgress{CompletedRoots: index + 1, TotalRoots: len(paths), CurrentRoot: m.FolderPath},
			)
		}
	}
	summary.ComponentCount = len(snapshot.Units)
	if summary.BlockedCount > 0 {
		summary.SummaryReason = "blocked"
	}
	snapshot.Summary, _ = json.Marshal(summary)
	return snapshot, nil
}

func fingerprint(template string, o Outcome, work Work, entries Directory, root string) string {
	values := map[string]string{"rj": work.Code}
	fields, _ := tokens(template)
	for _, key := range fields {
		if key != "rj" {
			values[key] = work.Values[key]
		}
	}
	source, exists := entries[o.SourceName]
	occupied := false
	for name := range entries {
		if pathnorm.RootPathKey(
			filepath.ToSlash(filepath.Join(root, name)),
		) == pathnorm.RootPathKey(
			filepath.ToSlash(filepath.Join(root, o.TargetName)),
		) {
			occupied = true
			break
		}
	}
	// Include only the target's occupancy, not unrelated library entries.
	b, _ := json.Marshal(struct {
		Code     string
		Values   map[string]string
		Source   Entry
		Exists   bool
		Occupied bool
	}{work.Code, values, source, exists, occupied})
	return workset.CanonicalJSONHash(b)
}

func planningScope(
	root string,
	d Draft,
	members []*workset.WorksetMember,
) ([]string, map[string]bool, []string, error) {
	paths := []string{}
	excluded := map[string]bool{}
	excludedIDs := []string{}
	for _, m := range d.Members {
		if m.Excluded {
			excluded[m.MemberID] = true
			excludedIDs = append(excludedIDs, m.MemberID)
		}
	}
	for _, m := range members {
		if pathnorm.RootPathKey(
			filepath.ToSlash(filepath.Dir(filepath.FromSlash(m.FolderPath))),
		) != pathnorm.RootPathKey(
			root,
		) {
			return nil, nil, nil, invalid("members must be immediate children of one library")
		}
		if !excluded[m.MemberID] {
			paths = append(paths, m.FolderPath)
		}
	}
	return paths, excluded, excludedIDs, nil
}

func freezeRoot(
	index int,
	m *workset.WorksetMember,
	template string,
	outcome Outcome,
	work Work,
	directory Directory,
	root string,
) workset.PlanRootFacts {
	facts := workset.PlanRootFacts{
		Index:                index,
		Path:                 m.FolderPath,
		Identity:             pathnorm.RootPathKey(m.FolderPath),
		InventoryFingerprint: fingerprint(template, outcome, work, directory, root),
		Count:                1,
		Status:               "ok",
	}
	if outcome.ReasonCode == "SOURCE_MISSING" {
		facts.Status = "missing"
		facts.Count = 0
		facts.ErrorCode = "SOURCE_MISSING"
		facts.ErrorMessage = "source directory is missing"
	}
	return facts
}
