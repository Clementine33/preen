package works

import (
	"context"
	"slices"
)

// PlanReportCleanup returns dependency-ordered, indivisible report groups.
// A nonnil eligible set bounds deletion to a discovery snapshot; reports outside
// it are retained along with their dependencies, including concurrent additions.
func PlanReportCleanup(ctx context.Context, ev Evidence, roots, eligible []WorkCode) ([][]WorkCode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolution := Resolve(ev)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	graph := resolution.Dependencies
	keep := map[WorkCode]bool{}
	queue := slices.Clone(roots)
	if eligible != nil {
		allowed := map[WorkCode]bool{}
		for _, id := range eligible {
			allowed[id] = true
		}
		for id := range ev.Self {
			if !allowed[id] {
				queue = append(queue, id)
			}
		}
	}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if keep[id] {
			continue
		}
		keep[id] = true
		queue = append(queue, graph[id]...)
	}
	return reportCleanupComponents(ctx, ev, graph, keep)
}

func reportCleanupComponents(
	ctx context.Context,
	ev Evidence,
	graph map[WorkCode][]WorkCode,
	keep map[WorkCode]bool,
) ([][]WorkCode, error) {
	// Kosaraju's source components put dependents before dependencies. Virtual
	// nodes participate in the graph but never count as reports.
	reverse := map[WorkCode][]WorkCode{}
	ids := make([]WorkCode, 0, len(graph))
	for id, deps := range graph {
		if keep[id] {
			continue
		}
		ids = append(ids, id)
		for _, dep := range deps {
			if !keep[dep] {
				reverse[dep] = append(reverse[dep], id)
			}
		}
	}
	slices.Sort(ids)
	visited := map[WorkCode]bool{}
	order := make([]WorkCode, 0, len(ids))
	var visit func(WorkCode) error
	visit = func(id WorkCode) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if visited[id] || keep[id] {
			return nil
		}
		visited[id] = true
		for _, dep := range graph[id] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		order = append(order, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	clear(visited)
	groups := [][]WorkCode{}
	var collect func(WorkCode, *[]WorkCode) error
	collect = func(id WorkCode, group *[]WorkCode) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if visited[id] {
			return nil
		}
		visited[id] = true
		if _, exists := ev.Self[id]; exists {
			*group = append(*group, id)
		}
		for _, dep := range reverse[id] {
			if err := collect(dep, group); err != nil {
				return err
			}
		}
		return nil
	}
	for _, id := range slices.Backward(order) {
		group := []WorkCode{}
		if err := collect(id, &group); err != nil {
			return nil, err
		}
		if len(group) > 0 {
			slices.Sort(group)
			groups = append(groups, group)
		}
	}
	return groups, nil
}
