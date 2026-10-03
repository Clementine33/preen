package refresh

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/onsei/organizer/backend/internal/works"
)

// Refresh validates its whole request before sending anything to an upstream.
func (s *Service) Refresh(ctx context.Context, req Request) (Result, error) {
	return s.RefreshWithProgress(ctx, req, nil)
}

// RefreshWithProgress notifies in serialized callbacks after each work's import.
// A callback failure stops admission and further imports, retaining committed results.
func (s *Service) RefreshWithProgress(
	ctx context.Context,
	req Request,
	progress func(WorkResult) error,
) (Result, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var notify sync.Mutex
	result := Result{Works: []WorkResult{}}
	ids, err := normalizeRequest(&req)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, id := range ids {
		item := WorkResult{WorkCode: id, Issues: []Issue{}}
		if _, detailErr := s.catalog.Detail(id); detailErr != nil {
			if detailErr.Kind == works.KindInternal {
				return result, detailErr
			}
			item.Issues = append(item.Issues, Issue{Code: detailErr.Code, Message: detailErr.Message})
		}
		result.Works = append(result.Works, item)
	}
	states, stateErr := s.states.MetadataStates(ctx, ids)
	if stateErr != nil {
		return result, stateErr
	}
	needs := make(map[works.WorkCode]map[works.DataGroup]bool, len(ids))
	dynamicIDs := make([]works.WorkCode, 0, len(ids))
	for _, item := range result.Works {
		want := wanted(req.Mode, states[item.WorkCode])
		needs[item.WorkCode] = want
		if len(item.Issues) == 0 && (want[works.GroupStatistics] || want[works.GroupRelations]) {
			dynamicIDs = append(dynamicIDs, item.WorkCode)
		}
	}
	dynamic, failures := s.dynamic(ctx, dynamicIDs, req.Locale)
	run := &refreshRun{service: s, dynamic: dynamic, finished: map[works.WorkCode][]Issue{}}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(8, len(result.Works)) {
		workers.Go(func() {
			for i := range jobs {
				item := &result.Works[i]
				if len(item.Issues) == 0 && ctx.Err() == nil {
					if _, e := s.refreshWork(
						ctx,
						req,
						item,
						needs[item.WorkCode],
						states[item.WorkCode][works.GroupCore].Source,
						dynamic[item.WorkCode],
						failures[item.WorkCode],
						run, false,
					); e != nil {
						cancel(e)
					}
				}
				if e := notifyWork(ctx, &notify, progress, *item); e != nil {
					cancel(e)
				}
			}
		})
	}
	for i := range result.Works {
		if ctx.Err() != nil {
			break
		}
		select {
		case jobs <- i:
		case <-ctx.Done():
		}
	}
	close(jobs)
	workers.Wait()

	return result, context.Cause(ctx)
}

func normalizeRequest(req *Request) ([]works.WorkCode, error) {
	if req.Mode == "" {
		req.Mode = FillMissing
	}
	if req.Mode != FillMissing && req.Mode != RefreshDynamic && req.Mode != RefreshAll {
		return nil, fmt.Errorf("unsupported refresh mode %q", req.Mode)
	}
	if req.Locale == "" {
		req.Locale = "ja-jp"
	}
	if req.Locale != "ja-jp" && req.Locale != "zh-cn" && req.Locale != "zh-tw" {
		return nil, fmt.Errorf("unsupported locale %q", req.Locale)
	}
	ids := make([]works.WorkCode, 0, len(req.WorkCodes))
	for _, raw := range req.WorkCodes {
		id, ok := works.ParseWorkCode(raw)
		if !ok {
			return nil, fmt.Errorf("invalid work number %q", raw)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one work number is required")
	}
	return ids, nil
}

func (s *Service) dynamic(
	ctx context.Context,
	ids []works.WorkCode,
	locale string,
) (map[works.WorkCode]works.ImportRequest, map[works.WorkCode]error) {
	out := map[works.WorkCode]works.ImportRequest{}
	failures := map[works.WorkCode]error{}
	for batch := range slices.Chunk(ids, 500) {
		if ctx.Err() != nil {
			break
		}
		data, err := s.dlsite.FetchDynamic(ctx, batch, locale)
		for _, id := range batch {
			if err != nil {
				failures[id] = err
				continue
			}
			if value, ok := data[id]; ok {
				out[id] = value
			} else {
				failures[id] = fmt.Errorf("work is absent from dynamic response")
			}
		}
	}
	return out, failures
}

func availableGroups(data works.ImportRequest) []works.DataGroup {
	out := []works.DataGroup{}
	if data.Core != nil {
		out = append(out, works.GroupCore)
	}
	if data.Statistics != nil {
		out = append(out, works.GroupStatistics)
	}
	if data.Relations != nil {
		out = append(out, works.GroupRelations)
	}
	if data.Editions != nil {
		out = append(out, works.GroupEditions)
	}
	return out
}

func wanted(mode Mode, states map[works.DataGroup]works.SourceState) map[works.DataGroup]bool {
	out := map[works.DataGroup]bool{}
	for _, group := range []works.DataGroup{works.GroupCore, works.GroupStatistics, works.GroupRelations, works.GroupEditions} {
		_, present := states[group]
		out[group] = mode == RefreshAll ||
			(mode == RefreshDynamic && (group == works.GroupStatistics || group == works.GroupRelations)) ||
			(mode == FillMissing && !present)
	}
	return out
}

func retain(data works.ImportRequest, want map[works.DataGroup]bool) works.ImportRequest {
	if !want[works.GroupCore] {
		data.Core = nil
	}
	if !want[works.GroupStatistics] {
		data.Statistics = nil
	}
	if !want[works.GroupRelations] {
		data.Relations = nil
	}
	if !want[works.GroupEditions] {
		data.Editions = nil
	}
	return data
}

func missingGroups(data works.ImportRequest, want map[works.DataGroup]bool) map[works.DataGroup]bool {
	out := map[works.DataGroup]bool{}
	for group, needed := range want {
		if needed {
			out[group] = true
		}
	}
	for _, group := range availableGroups(data) {
		delete(out, group)
	}
	return out
}

func notifyWork(ctx context.Context, lock *sync.Mutex, progress func(WorkResult) error, item WorkResult) error {
	if progress == nil {
		return nil
	}
	lock.Lock()
	defer lock.Unlock()
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return progress(item)
}
