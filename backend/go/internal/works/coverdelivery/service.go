package coverdelivery

import (
	"context"
	"fmt"
	"sync"
)

// Dependencies are the storage, CDN, filesystem and admission seams.
type Dependencies struct {
	Store      Store
	Downloader Downloader
	Source     Source
	Files      Files
	Staging    Staging
	Acquire    func() (func(), error)
	Refresh    func(context.Context, string, string) error
}
type activeDownload struct {
	job    Job
	cancel context.CancelFunc
}
type Service struct {
	deps Dependencies
	// Serialize candidate changes with publication; network transfers never hold this lock.
	publication sync.Mutex
	activeMu    sync.Mutex
	active      map[string]*activeDownload
}

func New(d Dependencies) (*Service, error) {
	if d.Store == nil || d.Downloader == nil || d.Files == nil || d.Staging == nil || d.Acquire == nil {
		return nil, fmt.Errorf("incomplete cover dependencies")
	}
	return &Service{deps: d, active: map[string]*activeDownload{}}, nil
}
