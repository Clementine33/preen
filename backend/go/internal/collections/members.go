package collections

import (
	"context"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

func (s *Service) Append(ctx context.Context, id string, raw []string) (AppendResult, *works.Error) {
	if len(raw) < 1 || len(raw) > works.MaxLimit {
		return AppendResult{}, works.Invalid(works.CodeInvalidArgument, "rjs must contain 1–200 work numbers")
	}
	ids := make([]works.WorkCode, 0, len(raw))
	seen := make(map[works.WorkCode]bool, len(raw))
	for _, value := range raw {
		code, ok := works.ParseWorkCode(value)
		if !ok {
			return AppendResult{}, works.Invalid(works.CodeInvalidArgument, "invalid work number: "+value)
		}
		if !seen[code] {
			seen[code] = true
			ids = append(ids, code)
		}
	}
	result, err := s.store.AppendCollectionWorks(ctx, id, ids, time.Now())
	return result, refusal(err)
}

func (s *Service) Remove(ctx context.Context, id string, work works.WorkCode) *works.Error {
	return refusal(s.store.RemoveCollectionWork(ctx, id, work, time.Now()))
}

func (s *Service) Move(ctx context.Context, id string, work works.WorkCode, before *string) *works.Error {
	var target *works.WorkCode
	if before != nil {
		code, ok := works.ParseWorkCode(*before)
		if !ok {
			return works.Invalid(works.CodeInvalidArgument, "invalid before_rj")
		}
		target = &code
	}
	return refusal(s.store.MoveCollectionWork(ctx, id, work, target, time.Now()))
}
