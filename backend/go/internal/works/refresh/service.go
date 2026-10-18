package refresh

import (
	"context"

	"github.com/Clementine33/preen/backend/internal/works"
)

// Catalog is the existing work catalog's read/write boundary.
type Catalog interface {
	Detail(works.WorkCode) (*works.WorkDetail, *works.Error)
	Import(works.WorkCode, works.ImportRequest) (works.ImportOutcome, *works.Error)
}

// States reads the persisted group presence used by missing-only refreshes.
type States interface {
	MetadataStates(context.Context, []works.WorkCode) (map[works.WorkCode]map[works.DataGroup]works.SourceState, error)
}

// Snapshot keeps a fetched main URL independent of whole-group validation.
type Snapshot struct {
	works.ImportRequest

	MainURL string
}

// DLsite exposes the two independent upstream channels.
type DLsite interface {
	FetchStatic(context.Context, works.WorkCode, string) (Snapshot, error)
	FetchDynamic(context.Context, []works.WorkCode, string) (map[works.WorkCode]works.ImportRequest, error)
}

// Backup supplies complete groups from the lower-priority source.
type Backup interface {
	FetchASMR(context.Context, works.WorkCode, string) (Snapshot, error)
}

// Service composes retrieval with the catalog's existing import rules.
type Service struct {
	catalog Catalog
	states  States
	dlsite  DLsite
	backup  Backup
}

// NewService creates a callable service; it starts no workers until Refresh is called.
func NewService(catalog Catalog, states States, dlsite DLsite, backup Backup) *Service {
	return &Service{catalog: catalog, states: states, dlsite: dlsite, backup: backup}
}
