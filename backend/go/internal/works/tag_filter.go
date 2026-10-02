package works

import (
	"sort"
	"strings"
)

// TagKind says which vocabulary a tag id belongs to: the source's tags, or the
// user's own.
type TagKind string

// The two tag vocabularies.
const (
	TagSource TagKind = "s"
	TagUser   TagKind = "u"
)

// TagRef is one tag id of either vocabulary.
type TagRef struct {
	Kind TagKind
	ID   string
}

// TagGroup is one tag the caller picked, as every id that spells it: a name
// carried by several entities (a scoped source tag per work, a user tag of the
// same name) is still one choice. A work matches a group when it carries any
// of its ids; TagMatch decides how the groups combine.
type TagGroup []TagRef

// ParseTagGroups reads the listing's `tag` parameter: groups separated by `,`,
// the refs of one group by `|`, each ref `s:<id>` or `u:<id>`. A malformed ref
// is refused, never dropped: dropping it would widen the answer.
func ParseTagGroups(raw string) ([]TagGroup, *Error) {
	var groups []TagGroup
	for part := range strings.SplitSeq(raw, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		var group TagGroup
		for ref := range strings.SplitSeq(part, "|") {
			kind, id, ok := strings.Cut(strings.TrimSpace(ref), ":")
			if !ok || id == "" || (TagKind(kind) != TagSource && TagKind(kind) != TagUser) {
				return nil, Invalid(CodeInvalidArgument, "tag refs must be s:<id> or u:<id>")
			}
			group = append(group, TagRef{Kind: TagKind(kind), ID: id})
		}
		groups = append(groups, group)
	}
	return groups, nil
}

// canonicalTagGroups is the order-free spelling of the groups the cursor
// digest carries: the same choice written in another order is the same query.
func canonicalTagGroups(groups []TagGroup) string {
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		refs := make([]string, 0, len(group))
		for _, ref := range group {
			refs = append(refs, string(ref.Kind)+":"+ref.ID)
		}
		sort.Strings(refs)
		out = append(out, strings.Join(refs, "|"))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
