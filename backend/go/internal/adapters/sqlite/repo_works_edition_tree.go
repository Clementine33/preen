package sqlite

import (
	"github.com/Clementine33/preen/backend/internal/works"
)

// loadEditionTree gathers the facts works.BuildEditionTree resolves: the known
// children of every candidate edition page (declared by the page, or resolved
// to it as parent) and whether each number involved is a work and held.
func (r *Repository) loadEditionTree(detail *works.WorkDetail) ([]works.EditionSlot, error) {
	self := detail.ID
	pages := map[works.WorkCode]bool{self: true}
	for _, e := range detail.Editions {
		pages[e.VersionRootRJ] = true
	}
	if detail.Relation.ParentID != nil {
		pages[*detail.Relation.ParentID] = true
	}
	pageIDs := make([]string, 0, len(pages))
	for page := range pages {
		pageIDs = append(pageIDs, string(page))
	}

	rows, err := r.db.Query(`
		SELECT declaring_work_id, child_work_id FROM work_declared_children
		WHERE declaring_work_id IN (`+placeholderList(len(pageIDs))+`)
		UNION
		SELECT parent_work_id, work_id FROM work_translations
		WHERE parent_work_id IN (`+placeholderList(len(pageIDs))+`)
	`, append(textArgs(pageIDs), textArgs(pageIDs)...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	children := map[works.WorkCode][]works.WorkCode{}
	known := map[string]bool{string(self): true}
	for _, id := range pageIDs {
		known[id] = true
	}
	if detail.Relation.OriginalID != nil {
		known[string(*detail.Relation.OriginalID)] = true
	}
	for rows.Next() {
		var page, child string
		if scanErr := rows.Scan(&page, &child); scanErr != nil {
			return nil, scanErr
		}
		children[works.WorkCode(page)] = append(children[works.WorkCode(page)], works.WorkCode(child))
		known[child] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	presenceRows, err := r.db.Query(`
		SELECT w.id, EXISTS (SELECT 1 FROM work_copies c WHERE c.work_id = w.id)
		FROM works w WHERE w.id IN (`+placeholderList(len(ids))+`)
	`, textArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer presenceRows.Close()
	presence := map[works.WorkCode]works.Presence{}
	for presenceRows.Next() {
		var id string
		var held bool
		if scanErr := presenceRows.Scan(&id, &held); scanErr != nil {
			return nil, scanErr
		}
		presence[works.WorkCode(id)] = works.Presence{IsWork: true, Held: held}
	}
	if err = presenceRows.Err(); err != nil {
		return nil, err
	}
	return works.BuildEditionTree(self, detail.Editions, detail.Relation, children, presence), nil
}
