package coverdelivery

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Dependencies are the storage, CDN, filesystem and admission seams.
type Dependencies struct {
	Store      Store
	Downloader Downloader
	Source     Source
	Files      Files
	Acquire    func() (func(), error)
	// BusyRetry is how long a download waits for file-operation admission
	// before delivering again; zero means one second.
	BusyRetry time.Duration
	// SyncInventory receives observed cover facts and exact removed names,
	// including partial publications. File may be nil for deletion-only recovery.
	// It never scans a member.
	SyncInventory func(context.Context, Target, Publication) error
	// RefreshSelected reconciles one execution's selected folders after metadata
	// and its cover demands finish. Acquire must exclude active task executions.
	RefreshSelected func(context.Context, string, []string) error
}
type activeDownload struct {
	ids    []string
	cancel context.CancelFunc
}
type Service struct {
	deps Dependencies
	// Serialize cancellation with publication; network transfers never hold this lock.
	publication sync.Mutex
	activeMu    sync.Mutex
	active      map[string]*activeDownload
	refreshMu   sync.Mutex
	refreshes   map[string]selectedRefresh
}

func New(d Dependencies) (*Service, error) {
	if d.Store == nil || d.Downloader == nil || d.Files == nil || d.Acquire == nil {
		return nil, fmt.Errorf("incomplete cover dependencies")
	}
	if d.BusyRetry <= 0 {
		d.BusyRetry = time.Second
	}
	return &Service{deps: d, active: map[string]*activeDownload{}, refreshes: map[string]selectedRefresh{}}, nil
}
