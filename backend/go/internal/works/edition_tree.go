package works

import "slices"

// The slot languages of the edition tree, in display order. Other stored
// languages are not shown: the tree is the Japanese original plus the two
// Chinese editions a collection holds.
const (
	TreeLanguageOriginal = "JPN"
	langChineseHans      = "CHI_HANS"
	langChineseHant      = "CHI_HANT"
)

// Roles a node plays in the edition tree.
const (
	TreeRoleOriginal    = "original"
	TreeRoleEditionPage = "edition_page"
	TreeRoleTranslation = "translation"
)

// Presence is what the catalog knows about one work number: whether a works
// row exists and whether it is held, i.e. has at
// least one Copy in any Library right now.
type Presence struct {
	IsWork bool
	Held   bool
}

// TreeNode is one work number in the edition tree.
type TreeNode struct {
	Presence

	RJ        WorkCode
	Role      string
	Language  *string
	Relation  ResolvedRelation
	SourceURL *string
}

// EditionSlot is one language line of the edition tree. Held is true when the
// root or any translation under it is held.
type EditionSlot struct {
	Language     string
	Label        *string
	Held         bool
	Root         TreeNode
	Translations []TreeNode
}

// BuildEditionTree resolves the work's language editions into at most three
// slots (JPN, CHI_HANS, CHI_HANT) from the union of its stored edition list,
// its resolved relation and the known children of each edition page
// (children: page -> resolved children). It returns nil when the
// tree holds no node besides the work itself.
func BuildEditionTree(
	self WorkCode,
	editions []LanguageEdition,
	relation ResolvedRelation,
	children map[WorkCode][]WorkCode,
	presence map[WorkCode]Presence,
) []EditionSlot {
	original := relation.OriginalID
	if relation.Role != nil && *relation.Role == RoleOriginal {
		original = &self
	}

	type page struct {
		root  WorkCode
		label *string
	}
	pages := map[string]page{}
	for _, e := range editions {
		if _, taken := pages[e.Language]; !taken && isChineseSlot(e.Language) {
			pages[e.Language] = page{e.VersionRootRJ, e.Label}
		}
	}
	if relation.Role != nil && relation.Language != nil && isChineseSlot(*relation.Language) {
		language := *relation.Language
		switch {
		case *relation.Role == RoleParent:
			pages[language] = page{self, pages[language].label}
		case *relation.Role == RoleChild && relation.ParentID != nil:
			pages[language] = page{*relation.ParentID, pages[language].label}
		}
	}

	node := func(rj WorkCode, role string) TreeNode {
		return TreeNode{RJ: rj, Role: role, Presence: presence[rj]}
	}
	var slots []EditionSlot
	if original != nil {
		root := node(*original, TreeRoleOriginal)
		slots = append(slots, EditionSlot{Language: TreeLanguageOriginal, Held: root.Held, Root: root})
	}
	for _, language := range []string{langChineseHans, langChineseHant} {
		p, ok := pages[language]
		if !ok {
			continue
		}
		kids := slices.Clone(children[p.root])
		if relation.Role != nil && *relation.Role == RoleChild && relation.ParentID != nil &&
			*relation.ParentID == p.root {
			kids = append(kids, self)
		}
		slices.Sort(kids)
		kids = slices.Compact(kids)
		slot := EditionSlot{Language: language, Label: p.label, Root: node(p.root, TreeRoleEditionPage)}
		slot.Held = slot.Root.Held
		for _, kid := range kids {
			if kid == p.root {
				continue
			}
			child := node(kid, TreeRoleTranslation)
			slot.Translations = append(slot.Translations, child)
			slot.Held = slot.Held || child.Held
		}
		slots = append(slots, slot)
	}

	nodes := 0
	for _, slot := range slots {
		nodes += 1 + len(slot.Translations)
	}
	if nodes == 0 || (nodes == 1 && slots[0].Root.RJ == self) {
		return nil
	}
	return slots
}

func isChineseSlot(language string) bool {
	return language == langChineseHans || language == langChineseHant
}
