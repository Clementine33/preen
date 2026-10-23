package refresh

import (
	"context"
	"fmt"

	"github.com/Clementine33/preen/backend/internal/works"
)

func (r *refreshRun) completeAncestors(ctx context.Context, req Request, item *WorkResult) error {
	// shortcut: serialize ancestor imports within one refresh, parallelize if parent chains become slow.
	r.upstream.Lock()
	defer r.upstream.Unlock()
	path := map[works.WorkCode]bool{item.WorkCode: true}
	issues, err := r.follow(ctx, req, item.WorkCode, path)
	item.Issues = append(item.Issues, issues...)
	return err
}

func (r *refreshRun) follow(
	ctx context.Context,
	req Request,
	subject works.WorkCode,
	path map[works.WorkCode]bool,
) ([]Issue, error) {
	if ctx.Err() != nil {
		return nil, nil
	}
	parent, err := r.service.states.RelationParent(ctx, subject)
	if err != nil || parent == nil {
		return nil, err
	}
	id := *parent
	if path[id] {
		return []Issue{{Code: works.CodeRelationCycle, Message: "upstream cycle at " + string(id)}}, nil
	}
	if previous, done := r.finished[id]; done {
		return previous, nil
	}
	path[id] = true
	issues, err := r.refreshAncestor(ctx, req, id)
	if err == nil {
		more, followErr := r.follow(ctx, req, id, path)
		issues = append(issues, more...)
		err = followErr
	}
	delete(path, id)
	if err == nil {
		r.finished[id] = issues
	}
	return issues, err
}

// refreshAncestor imports only relation evidence; source pages never become catalog works.
func (r *refreshRun) refreshAncestor(ctx context.Context, req Request, id works.WorkCode) ([]Issue, error) {
	states, err := r.service.states.MetadataStates(ctx, []works.WorkCode{id})
	if err != nil {
		return nil, err
	}
	if !wanted(req.Mode, states[id])[works.GroupRelations] {
		return nil, nil
	}
	issues := []Issue{}
	data := r.dynamic[id]
	if data.Relations == nil {
		fetched, fetchErr := r.service.dlsite.FetchDynamic(ctx, []works.WorkCode{id}, req.Locale)
		if fetchErr != nil {
			issues = append(
				issues,
				Issue{Source: works.SourceDLsite, Code: "DYNAMIC_FAILED", Message: fetchErr.Error()},
			)
		} else {
			data = fetched[id]
		}
	}
	if data.Relations == nil {
		fallback, fetchErr := r.fetch(ctx, id, works.SourceASMRone, req.Locale)
		if fetchErr != nil {
			issues = append(
				issues,
				Issue{Source: works.SourceASMRone, Code: "BACKUP_FAILED", Message: fetchErr.Error()},
			)
		} else {
			data.Relations = fallback.Relations
		}
	}
	data = chineseSnapshot(data)
	if ctx.Err() != nil {
		return nil, nil
	}
	if data.Relations == nil {
		issues = append(
			issues,
			Issue{
				Group:   works.GroupRelations,
				Code:    "GROUP_UNAVAILABLE",
				Message: "no complete snapshot was retrieved",
			},
		)
	} else if _, refusal := r.service.catalog.ImportRelationReport(id, *data.Relations); refusal != nil {
		if refusal.Kind == works.KindInternal {
			return issues, refusal
		}
		issues = append(issues, Issue{Group: works.GroupRelations, Code: refusal.Code, Message: refusal.Message})
	}
	for i := range issues {
		issues[i].Message = fmt.Sprintf("upstream %s: %s", id, issues[i].Message)
	}
	return issues, nil
}
