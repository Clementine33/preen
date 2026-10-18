package refresh

import (
	"context"
	"fmt"

	"github.com/Clementine33/preen/backend/internal/works"
)

func (r *refreshRun) completeAncestors(
	ctx context.Context, req Request, item *WorkResult, report *works.ImportRelations,
) error {
	// ponytail: serialize upstream imports per refresh; parallelize discovery if large families become slow.
	r.upstream.Lock()
	defer r.upstream.Unlock()
	path := map[works.WorkCode]bool{item.WorkCode: true}
	issues, err := r.follow(ctx, req, report, path)
	item.Issues = append(item.Issues, issues...)
	return err
}

func (r *refreshRun) follow(
	ctx context.Context, req Request, report *works.ImportRelations, path map[works.WorkCode]bool,
) ([]Issue, error) {
	if report == nil || report.Role == "original" {
		return nil, nil
	}
	issues := []Issue{}
	for _, raw := range []*string{report.ParentRJ, report.OriginalRJ} {
		if raw == nil || ctx.Err() != nil {
			continue
		}
		id, ok := works.ParseWorkCode(*raw)
		if !ok {
			continue // The source snapshot validator already refused malformed references.
		}
		if path[id] {
			issues = append(issues, Issue{Code: works.CodeRelationCycle, Message: "upstream cycle at " + string(id)})
			continue
		}
		if previous, done := r.finished[id]; done {
			issues = append(issues, previous...)
			continue
		}
		path[id] = true
		found, err := r.refreshAncestor(ctx, req, id, path)
		delete(path, id)
		if err != nil {
			return issues, err
		}
		r.finished[id] = found
		issues = append(issues, found...)
	}
	return issues, nil
}

func (r *refreshRun) refreshAncestor(
	ctx context.Context, req Request, id works.WorkCode, path map[works.WorkCode]bool,
) ([]Issue, error) {
	states, err := r.service.states.MetadataStates(ctx, []works.WorkCode{id})
	if err != nil {
		return nil, err
	}
	want := wanted(req.Mode, states[id])
	want[works.GroupStatistics] = false
	dynamic := r.dynamic[id]
	var failure error
	if want[works.GroupRelations] && dynamic.Relations == nil {
		fetched, fetchErr := r.service.dlsite.FetchDynamic(ctx, []works.WorkCode{id}, req.Locale)
		failure = fetchErr
		dynamic = fetched[id]
	}
	item := WorkResult{WorkCode: id}
	report, err := r.service.refreshWork(ctx, req, &item, want, states[id][works.GroupCore].Source,
		dynamic, failure, r, true)
	if err != nil {
		return nil, err
	}
	for i := range item.Issues {
		item.Issues[i].Message = fmt.Sprintf("upstream %s: %s", id, item.Issues[i].Message)
	}
	more, err := r.follow(ctx, req, report, path)
	return append(item.Issues, more...), err
}
