package metadata

import (
	"context"
	"slices"

	"github.com/onsei/organizer/backend/internal/works"
)

func (s *Service) refreshWork(
	ctx context.Context,
	req Request,
	item *WorkResult,
	want map[works.DataGroup]bool,
	dynamic works.ImportRequest,
	failure error,
) {
	data := works.ImportRequest{}
	if want[works.GroupCore] || want[works.GroupEditions] {
		snapshot, err := s.dlsite.FetchStatic(ctx, item.WorkCode, req.Locale)
		if err != nil {
			item.Issues = append(
				item.Issues,
				Issue{Source: works.SourceDLsite, Code: "STATIC_FAILED", Message: err.Error()},
			)
		} else {
			data = snapshot
		}
	}
	if ctx.Err() != nil {
		return
	}
	data.Statistics = dynamic.Statistics
	if dynamic.Relations != nil {
		data.Relations = dynamic.Relations
	}
	if failure != nil {
		item.Issues = append(
			item.Issues,
			Issue{Source: works.SourceDLsite, Code: "DYNAMIC_FAILED", Message: failure.Error()},
		)
	}
	data = s.supplyMissing(ctx, req, item, data, want)
	if ctx.Err() != nil {
		return
	}
	missing := missingGroups(data, want)
	groups := make([]works.DataGroup, 0, len(missing))
	for group := range missing {
		groups = append(groups, group)
	}
	slices.Sort(groups)
	for _, group := range groups {
		item.Issues = append(
			item.Issues,
			Issue{Group: group, Code: "GROUP_UNAVAILABLE", Message: "no complete snapshot was retrieved"},
		)
	}
	data = retain(data, want)
	if len(availableGroups(data)) == 0 {
		return
	}
	outcome, err := s.catalog.Import(item.WorkCode, data)
	item.Outcome = outcome
	if err != nil {
		item.Issues = append(item.Issues, Issue{Code: err.Code, Message: err.Message})
	}
}

func (s *Service) supplyMissing(
	ctx context.Context,
	req Request,
	item *WorkResult,
	data works.ImportRequest,
	want map[works.DataGroup]bool,
) works.ImportRequest {
	missing := missingGroups(data, want)
	if !missing[works.GroupCore] && !missing[works.GroupRelations] && !missing[works.GroupEditions] {
		return data
	}
	fallback, err := s.backup.FetchASMR(ctx, item.WorkCode, req.Locale)
	if err != nil {
		item.Issues = append(
			item.Issues,
			Issue{Source: works.SourceASMRone, Code: "BACKUP_FAILED", Message: err.Error()},
		)
		return data
	}
	if missing[works.GroupCore] {
		data.Core = fallback.Core
	}
	if missing[works.GroupRelations] {
		data.Relations = fallback.Relations
	}
	if missing[works.GroupEditions] {
		data.Editions = fallback.Editions
	}
	return data
}
