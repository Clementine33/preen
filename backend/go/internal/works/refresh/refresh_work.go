package refresh

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
	storedSource works.Source,
	dynamic works.ImportRequest,
	failure error,
	run *refreshRun,
	ancestor bool,
) (*works.ImportRelations, error) {
	data := works.ImportRequest{}
	if want[works.GroupCore] || want[works.GroupEditions] {
		snapshot, err := run.localized(ctx, item.WorkCode, works.SourceDLsite, req.Locale, dynamic.Relations)
		data = snapshot.ImportRequest
		item.MainURL = snapshot.MainURL
		if err != nil {
			item.Issues = append(
				item.Issues,
				Issue{Source: works.SourceDLsite, Code: "STATIC_FAILED", Message: err.Error()},
			)
		} else {
			item.MainChecked = true
		}
	}
	if ctx.Err() != nil {
		return nil, nil
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
	if data.Core != nil && data.Core.CoverURL != nil {
		item.MainURL = *data.Core.CoverURL
	}
	data = s.supplyMissing(ctx, req, item, data, want, run)
	data = chineseSnapshot(data)
	if want[works.GroupCore] {
		if err := run.originalCircle(ctx, data); err != nil {
			data.Core = nil
			item.Issues = append(
				item.Issues,
				Issue{Group: works.GroupCore, Code: "ORIGINAL_UNAVAILABLE", Message: err.Error()},
			)
		}
	}
	if item.MainURL == "" && !want[works.GroupCore] {
		saved, e := s.catalog.Detail(item.WorkCode)
		if e != nil && e.Kind == works.KindInternal {
			return nil, e
		}
		if e == nil && saved.CoverURL != nil {
			if storedSource == works.SourceDLsite {
				item.MainURL = *saved.CoverURL
			} else if item.BackupURL == "" {
				item.BackupURL = *saved.CoverURL
			}
		}
	}
	if ctx.Err() != nil {
		return nil, nil
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
	report := data.Relations
	data = retain(data, want)
	if len(availableGroups(data)) == 0 {
		return nil, nil
	}
	outcome, err := s.catalog.Import(item.WorkCode, data)
	item.Outcome = outcome
	if err != nil {
		item.Issues = append(item.Issues, Issue{Code: err.Code, Message: err.Message})
		if err.Kind == works.KindInternal {
			return nil, err
		}
		return nil, nil
	}
	if !ancestor {
		if e := run.completeAncestors(ctx, req, item, report); e != nil {
			return report, e
		}
	}
	return report, nil
}

func (s *Service) supplyMissing(
	ctx context.Context,
	req Request,
	item *WorkResult,
	data works.ImportRequest,
	want map[works.DataGroup]bool,
	run *refreshRun,
) works.ImportRequest {
	missing := missingGroups(data, want)
	if !missing[works.GroupCore] && !missing[works.GroupRelations] && !missing[works.GroupEditions] {
		return data
	}
	fallback, err := run.localized(ctx, item.WorkCode, works.SourceASMRone, req.Locale, data.Relations)
	if err != nil {
		item.Issues = append(
			item.Issues,
			Issue{Source: works.SourceASMRone, Code: "BACKUP_FAILED", Message: err.Error()},
		)
		return data
	}
	item.BackupChecked = true
	item.BackupURL = fallback.MainURL
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
