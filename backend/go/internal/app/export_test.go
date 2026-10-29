package app

import (
	"context"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

// StartRetiredPlanCleanup exposes the process lifecycle to integration tests.
func StartRetiredPlanCleanup(ctx context.Context, repo *sqlite.Repository) (func(), <-chan struct{}) {
	c, done := startRetiredPlans(ctx, repo)
	return c.kick, done
}

// NewOutboundProxy exposes the proxy holder to the black-box tests.
var NewOutboundProxy = newOutboundProxy

// WiredMediaFacts exposes the process's assembled media pass to integration tests.
func WiredMediaFacts(repo *sqlite.Repository, dir string) *inventory.MediaEnricher {
	return buildServices(repo, dir, dir).mediaFacts
}
