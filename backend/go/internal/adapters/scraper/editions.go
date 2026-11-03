package scraper

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Clementine33/preen/backend/internal/works"
)

// resolveDuplicateEditions settles a language DLsite lists more than once by
// looking each candidate up: one still listed with the fetched work's type wins
// (e.g. the audio release over a script-only PDF). Languages it cannot settle,
// including those listing the fetched work itself, are left to
// oneEditionPerLanguage.
func (c *Client) resolveDuplicateEditions(
	ctx context.Context,
	id works.WorkCode,
	workType *string,
	group *works.ImportEditions,
	locale string,
) {
	if group == nil {
		return
	}
	byLanguage := map[string][]int{}
	for i, item := range group.Items {
		byLanguage[item.Language] = append(byLanguage[item.Language], i)
	}
	drop := map[int]bool{}
	for _, candidates := range byLanguage {
		if len(candidates) < 2 || listsWork(group.Items, candidates, id) {
			continue
		}
		best, bestScore := -1, -1
		for _, i := range candidates {
			score, err := c.editionScore(ctx, group.Items[i].VersionRootRJ, workType, locale)
			if err != nil {
				best = -1
				break
			}
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			continue
		}
		for _, i := range candidates {
			drop[i] = i != best
		}
	}
	items := []works.ImportEditionItem{}
	for i, item := range group.Items {
		if !drop[i] {
			items = append(items, item)
		}
	}
	group.Items = items
}

func listsWork(items []works.ImportEditionItem, candidates []int, id works.WorkCode) bool {
	for _, i := range candidates {
		if root, ok := works.ParseWorkCode(items[i].VersionRootRJ); ok && root == id {
			return true
		}
	}
	return false
}

// editionScore ranks a candidate: 0 when DLsite no longer lists it, 1 when
// listed, 2 when listed with the same work type as the fetched work.
func (c *Client) editionScore(ctx context.Context, raw string, workType *string, locale string) (int, error) {
	code, ok := works.ParseWorkCode(raw)
	if !ok {
		return 0, fmt.Errorf("edition %q is not a work number", raw)
	}
	var items []record
	query := url.Values{"workno": {requestCode(code)}, "locale": {locale}}
	if err := c.get(ctx, "https://www.dlsite.com/maniax/api/=/product.json?"+query.Encode(), &items); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	actual := scalar[string](items[0], "workno")
	if actual == nil {
		return 0, fmt.Errorf("edition response has no work number")
	}
	if got, ok := works.ParseWorkCode(*actual); !ok || got != code {
		return 0, fmt.Errorf("edition response belongs to another work")
	}
	other := scalar[string](items[0], "work_type")
	if workType != nil && other != nil && *other == *workType {
		return 2, nil
	}
	return 1, nil
}
