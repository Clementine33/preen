package app

import (
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

// NewOutboundProxy exposes the proxy holder to the black-box tests.
var NewOutboundProxy = newOutboundProxy

// WiredMediaFacts exposes the process's assembled media pass to integration tests.
func WiredMediaFacts(repo *sqlite.Repository, dir string) *inventory.MediaEnricher {
	return buildServices(repo, dir, dir).mediaFacts
}
