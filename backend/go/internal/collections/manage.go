package collections

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

func (s *Service) List(ctx context.Context, window Window) (Page, *works.Error) {
	if invalid := window.Validate(); invalid != nil {
		return Page{}, invalid
	}
	page, err := s.store.ListCollections(ctx, window)
	return page, refusal(err)
}

func (s *Service) Create(ctx context.Context, name string) (Collection, *works.Error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Collection{}, works.Invalid(works.CodeInvalidArgument, "name must not be blank")
	}
	collection, err := s.store.CreateCollection(ctx, name, time.Now())
	return collection, refusal(err)
}

func (s *Service) Rename(ctx context.Context, id, name string) (Collection, *works.Error) {
	if id == FavoritesID {
		return Collection{}, builtinRefusal()
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Collection{}, works.Invalid(works.CodeInvalidArgument, "name must not be blank")
	}
	c, err := s.store.RenameCollection(ctx, id, name, time.Now())
	return c, refusal(err)
}

func (s *Service) Delete(ctx context.Context, id string) *works.Error {
	if id == FavoritesID {
		return builtinRefusal()
	}
	return refusal(s.store.DeleteCollection(ctx, id))
}

func builtinRefusal() *works.Error {
	return works.NewError(works.KindConflict, CodeBuiltin, "Favorites cannot be renamed or deleted")
}

func refusal(err error) *works.Error {
	if err == nil {
		return nil
	}
	if business, ok := errors.AsType[*works.Error](err); ok {
		return business
	}
	return works.NewError(works.KindInternal, "INTERNAL", "collection storage failed")
}

func (s *Service) Detail(ctx context.Context, id string, window Window) (Detail, *works.Error) {
	if invalid := window.Validate(); invalid != nil {
		return Detail{}, invalid
	}
	detail, err := s.store.GetCollection(ctx, id, window)
	return detail, refusal(err)
}
