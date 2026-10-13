package collections

import (
	"context"
	"time"

	"github.com/onsei/organizer/backend/internal/works"
)

// Service validates collection intents and delegates atomic persistence to Store.
type Service struct{ store Store }

// Store keeps collection reads and writes atomic at their business boundaries.
type Store interface {
	ListCollections(context.Context, Window) (Page, error)
	CreateCollection(context.Context, string, time.Time) (Collection, error)
	RenameCollection(context.Context, string, string, time.Time) (Collection, error)
	DeleteCollection(context.Context, string) error
	GetCollection(context.Context, string, Window) (Detail, error)
	AppendCollectionWorks(context.Context, string, []works.WorkCode, time.Time) (AppendResult, error)
	RemoveCollectionWork(context.Context, string, works.WorkCode, time.Time) error
	MoveCollectionWork(context.Context, string, works.WorkCode, *works.WorkCode, time.Time) error
}

func NewService(store Store) *Service { return &Service{store: store} }
