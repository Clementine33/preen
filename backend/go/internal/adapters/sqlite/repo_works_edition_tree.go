package sqlite

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/Clementine33/preen/backend/internal/works"
)

// workDetailReader keeps every detail query on the same read snapshot.
type workDetailReader interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

// loadRelationDetail resolves external and local nodes from the detail snapshot.
func loadRelationDetail(tx *sql.Tx, detail *works.WorkDetail) error {
	evidence, err := loadEvidence(tx)
	if err != nil {
		return err
	}
	resolution := works.Resolve(evidence)
	children := map[works.WorkCode][]works.WorkCode{}
	for child, relation := range resolution.Relations {
		if relation.ParentID != nil {
			children[*relation.ParentID] = append(children[*relation.ParentID], child)
		}
	}
	tree := works.BuildEditionTree(detail.ID, detail.Editions, detail.Relation, children, nil)
	known := map[works.WorkCode]bool{detail.ID: true}
	for _, edition := range detail.Editions {
		known[edition.VersionRootRJ] = true
	}
	// Follow effective parents without skipping uncollected or language-free bridges.
	visited := map[works.WorkCode]bool{detail.ID: true}
	for id := detail.Relation.ParentID; id != nil && !visited[*id]; id = resolution.Relations[*id].ParentID {
		visited[*id], known[*id] = true, true
	}
	if detail.Relation.OriginalID != nil {
		known[*detail.Relation.OriginalID] = true
	}
	for _, slot := range tree {
		known[slot.Root.RJ] = true
		for _, node := range slot.Translations {
			known[node.RJ] = true
		}
	}
	warningScope := map[works.WorkCode]bool{}
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, string(id))
		warningScope[id] = true
		// Report rejected claims separately, never as normal tree edges.
		for _, child := range evidence.DeclaredChildren[id] {
			warningScope[child] = true
		}
	}
	for _, warning := range resolution.Warnings {
		if warningScope[warning.WorkID] {
			detail.RelationWarnings = append(detail.RelationWarnings, warning)
		}
	}
	nodes, err := loadRelationNodes(tx, ids, resolution.Relations, detail.Editions)
	if err != nil {
		return err
	}
	slices.Sort(ids)
	for _, id := range ids {
		detail.RelationNodes = append(detail.RelationNodes, nodes[works.WorkCode(id)])
	}
	enrich := func(node works.TreeNode) works.TreeNode {
		enriched := nodes[node.RJ]
		enriched.Role = node.Role
		return enriched
	}
	for i := range tree {
		slot := &tree[i]
		slot.Root = enrich(slot.Root)
		slot.Held = slot.Root.Held
		for j := range slot.Translations {
			slot.Translations[j] = enrich(slot.Translations[j])
			slot.Held = slot.Held || slot.Translations[j].Held
		}
	}
	detail.EditionTree = tree
	return nil
}

func loadRelationNodes(
	tx *sql.Tx,
	ids []string,
	relations map[works.WorkCode]works.ResolvedRelation,
	editions []works.LanguageEdition,
) (map[works.WorkCode]works.TreeNode, error) {
	nodes := map[works.WorkCode]works.TreeNode{}
	for _, raw := range ids {
		id := works.WorkCode(raw)
		relation := relations[id]
		role := ""
		if relation.Role != nil {
			role = string(*relation.Role)
		}
		nodes[id] = works.TreeNode{
			RJ:        id,
			Role:      role,
			Relation:  relation,
			Language:  relation.Language,
			SourceURL: relationSourceURL(id),
		}
	}
	for _, edition := range editions {
		node := nodes[edition.VersionRootRJ]
		if node.Language == nil {
			node.Language = new(edition.Language)
		}
		nodes[node.RJ] = node
	}
	rows, err := tx.Query(`SELECT w.id, w.language, EXISTS (SELECT 1 FROM work_copies c WHERE c.work_id = w.id)
        FROM works w WHERE w.id IN (`+placeholderList(len(ids))+`)`, textArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id works.WorkCode
		var language sql.NullString
		var held bool
		if err := rows.Scan(&id, &language, &held); err != nil {
			return nil, err
		}
		node := nodes[id]
		node.Presence = works.Presence{IsWork: true, Held: held}
		if language.Valid {
			node.Language = nullString(language)
		}
		nodes[id] = node
	}
	return nodes, rows.Err()
}

// relationSourceURL uses the supported source's identity, never fetched URL text.
func relationSourceURL(id works.WorkCode) *string {
	canonical, valid := works.ParseWorkCode(string(id))
	if !valid {
		return nil
	}
	sourceID := string(canonical)
	if strings.HasPrefix(sourceID, "RJ00") {
		sourceID = "RJ" + sourceID[4:]
	}
	return new("https://www.dlsite.com/maniax/work/=/product_id/" + sourceID + ".html")
}
