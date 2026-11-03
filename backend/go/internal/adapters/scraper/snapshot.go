package scraper

import (
	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

// Incomplete groups stay absent, so another source can supply them
// independently; the reason each was dropped is kept on the snapshot.
func validSnapshot(id works.WorkCode, snap refresh.Snapshot) refresh.Snapshot {
	check := func(group works.DataGroup, req works.ImportRequest) bool {
		_, err := works.ValidateImport(id, req)
		if err != nil {
			reject(&snap, group, err.Error())
		}
		return err == nil
	}
	if snap.Core != nil && !check(works.GroupCore, works.ImportRequest{Core: snap.Core}) {
		snap.Core = nil
	}
	if snap.Statistics != nil && !check(works.GroupStatistics, works.ImportRequest{Statistics: snap.Statistics}) {
		snap.Statistics = nil
	}
	if snap.Relations != nil && !check(works.GroupRelations, works.ImportRequest{Relations: snap.Relations}) {
		snap.Relations = nil
	}
	if snap.Editions != nil {
		snap.Editions = oneEditionPerLanguage(id, *snap.Editions)
		if !check(works.GroupEditions, works.ImportRequest{Editions: snap.Editions}) {
			snap.Editions = nil
		}
	}
	return snap
}

func reject(snap *refresh.Snapshot, group works.DataGroup, reason string) {
	if snap.Rejected == nil {
		snap.Rejected = make(map[works.DataGroup]string)
	}
	snap.Rejected[group] = reason
}

// oneEditionPerLanguage keeps one edition per language, as the domain requires,
// for duplicates no lookup settled: the fetched work's own entry wins, otherwise
// the first listed.
func oneEditionPerLanguage(id works.WorkCode, group works.ImportEditions) *works.ImportEditions {
	at := map[string]int{}
	items := []works.ImportEditionItem{}
	for _, item := range group.Items {
		i, dup := at[item.Language]
		if !dup {
			at[item.Language] = len(items)
			items = append(items, item)
		} else if root, ok := works.ParseWorkCode(item.VersionRootRJ); ok && root == id {
			items[i] = item
		}
	}
	group.Items = items
	return &group
}
