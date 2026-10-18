package dirrename

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Clementine33/preen/backend/internal/librarypath"
	"github.com/Clementine33/preen/backend/internal/workset"
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
	// CustomName is the literal name the user gave this folder instead of the template.
	CustomName string `json:"custom_name,omitempty"`
	// ObservedName is what a hand-renamed folder is called on disk now; the
	// execution first moves the stored paths to it, then renames it.
	ObservedName string `json:"observed_name,omitempty"`
	// RebindTo is the work code a custom name carries when it differs from the
	// bound work: the copy follows the name.
	RebindTo string `json:"rebind_to,omitempty"`
}

func (t *Task) PlanSession(ctx context.Context, in workset.PlanSessionInput) (*workset.PlanSnapshot, error) {
	if err := t.ValidateSessionInput(in.RawDraft, in.Members); err != nil {
		return nil, err
	}
	d, _ := decodeDraft(in.RawDraft)
	root := librarypath.CleanRootPath(in.LibraryRoot)
	paths, excluded, excludedIDs, err := planningScope(root, d, in.Members)
	if err != nil {
		return nil, err
	}

	works, err := t.catalog.RenameWorks(ctx, root, paths)
	if err != nil {
		return nil, err
	}
	directories, readErrors := t.readParentDirectories(ctx, paths)
	for _, folder := range paths {
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(folder)))
		if readErr := readErrors[parent]; readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			return nil, readErr
		}
	}
	observed, err := t.followHandRenames(ctx, root, in.Members, excluded, directories, works)
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
	outcomes, targets := planNames(d, in.Members, excluded, works, directories, observed)
	for _, m := range in.Members {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if excluded[m.MemberID] {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(m.FolderPath)))
		outcome := outcomes[m.MemberID]
		if outcome.ReasonCode == "" &&
			targets[librarypath.RootPathKey(filepath.ToSlash(filepath.Join(parent, outcome.TargetName)))] > 1 {
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
		facts := freezeRoot(index, m, d.Common, outcome, works[m.FolderPath], directories[parent], parent)
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

func planNames(
	d Draft,
	members []*workset.WorksetMember,
	excluded map[string]bool,
	works map[string]Work,
	directories map[string]Directory,
	observed map[string]string,
) (map[string]Outcome, map[string]int) {
	custom := map[string]string{}
	for _, dm := range d.Members {
		if dm.Name != "" {
			custom[dm.MemberID] = dm.Name
		}
	}
	outcomes := map[string]Outcome{}
	targets := map[string]int{}
	for _, m := range members {
		if excluded[m.MemberID] {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(m.FolderPath)))
		o := planName(d.Common, m, works[m.FolderPath], directories[parent], parent,
			naming{Custom: custom[m.MemberID], Observed: observed[m.MemberID]})
		outcomes[m.MemberID] = o
		if o.ReasonCode == "" {
			targets[librarypath.RootPathKey(filepath.ToSlash(filepath.Join(parent, o.TargetName)))]++
		}
	}
	return outcomes, targets
}

func fingerprint(common Common, o Outcome, work Work, entries Directory, root string) string {
	values := map[string]string{"rj": work.Code}
	fields, _ := tokens(common.Template)
	for _, key := range fields {
		if key != "rj" {
			values[key] = work.field(key, common)
		}
	}
	disk := o.SourceName
	if o.ObservedName != "" {
		disk = o.ObservedName
	}
	source, exists := entries[disk]
	source.Modified = 0 // Directory mtime describes child changes, not a rename input.
	occupied := false
	for name := range entries {
		if librarypath.RootPathKey(
			filepath.ToSlash(filepath.Join(root, name)),
		) == librarypath.RootPathKey(
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
		Custom   string
		Observed string
	}{work.Code, values, source, exists, occupied, o.CustomName, o.ObservedName})
	return workset.CanonicalJSONHash(b)
}

func planningScope(
	root string,
	d Draft,
	members []*workset.WorksetMember,
) ([]string, map[string]bool, []string, error) {
	if len(members) == 0 || strings.TrimSpace(root) == "" || root == "." {
		return nil, nil, nil, invalid("library root and members are required")
	}
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
		rel, ok := librarypath.RelPath(m.RelPath)
		if !ok || librarypath.RootPathKey(librarypath.JoinRel(root, rel)) != librarypath.RootPathKey(m.FolderPath) {
			return nil, nil, nil, invalid("members must be beneath the saved library root")
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
	common Common,
	outcome Outcome,
	work Work,
	directory Directory,
	root string,
) workset.PlanRootFacts {
	facts := workset.PlanRootFacts{
		Index:                index,
		Path:                 m.FolderPath,
		Identity:             librarypath.RootPathKey(m.FolderPath),
		InventoryFingerprint: fingerprint(common, outcome, work, directory, root),
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

// readParentDirectories snapshots occupancy once for each participating parent.
func (t *Task) readParentDirectories(ctx context.Context, paths []string) (map[string]Directory, map[string]error) {
	directories := map[string]Directory{}
	readErrors := map[string]error{}
	for _, folder := range paths {
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(folder)))
		if _, ok := directories[parent]; ok {
			continue
		}
		directories[parent], readErrors[parent] = t.readDirectory(ctx, parent)
	}
	return directories, readErrors
}

// followHandRenames finds, for each participating member whose recorded folder
// is gone, the directory a hand rename left in the same parent. It reads no
// disk beyond the parent listings planning already holds; the work metadata of
// a followed folder falls back to the one still bound to the recorded path, so
// it also works before any rescan. works is completed in place.
func (t *Task) followHandRenames(
	ctx context.Context,
	root string,
	members []*workset.WorksetMember,
	excluded map[string]bool,
	directories map[string]Directory,
	works map[string]Work,
) (map[string]string, error) {
	taken := map[string]map[string]bool{}
	for _, m := range members {
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(m.FolderPath)))
		if taken[parent] == nil {
			taken[parent] = map[string]bool{}
		}
		taken[parent][filepath.Base(filepath.FromSlash(m.FolderPath))] = true
	}
	observed := map[string]string{}
	var lookups []string
	for _, m := range members {
		if excluded[m.MemberID] {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(m.FolderPath)))
		name := followed(filepath.Base(filepath.FromSlash(m.FolderPath)), directories[parent], taken[parent])
		if name == "" {
			continue
		}
		observed[m.MemberID] = name
		lookups = append(lookups, librarypath.JoinRel(parent, name))
	}
	if len(lookups) == 0 {
		return observed, nil
	}
	found, err := t.catalog.RenameWorks(ctx, root, lookups)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		name := observed[m.MemberID]
		if name == "" {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(m.FolderPath)))
		if w, ok := found[librarypath.JoinRel(parent, name)]; ok {
			works[m.FolderPath] = w
		}
	}
	return observed, nil
}
