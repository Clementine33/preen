package updatecheck

import (
	"context"
	"sync"
	"time"
)

// States a Status can be in.
const (
	// StateChecking means the single check has not settled yet.
	StateChecking = "checking"
	// StateAvailable means a newer stable release exists.
	StateAvailable = "available"
	// StateNone means no newer release is known: the running version is the
	// latest, the check was skipped, or it failed.
	StateNone = "none"
)

// Status is the result of the check. LatestVersion and ReleaseURL are set only
// when State is StateAvailable.
type Status struct {
	State         string
	LatestVersion string
	ReleaseURL    string
}

// Release is the latest stable release a Source reports: its tag (such as
// "v0.2.0") and the page that describes it.
type Release struct {
	Tag string
	URL string
}

// Source reads the latest stable release once. An error it marks with
// Transient is worth another attempt; any other error ends the check.
type Source func(ctx context.Context) (Release, error)

// Service holds the check's result and runs the check.
type Service struct {
	current string
	source  Source
	sleep   func(ctx context.Context, d time.Duration) error

	mu     sync.Mutex
	status Status
}

// Option adjusts a Service; tests use it to avoid real waiting.
type Option func(*Service)

// WithSleep replaces the wait between attempts. It returns early with the
// context's error when ctx is done.
func WithSleep(sleep func(ctx context.Context, d time.Duration) error) Option {
	return func(s *Service) { s.sleep = sleep }
}

// New returns a Service for the running version. A nil source, or a version
// that is not a semantic version (such as a plain "dev"), skips the check: the
// status is StateNone at once and Run returns immediately.
func New(current string, source Source, opts ...Option) *Service {
	s := &Service{current: current, source: source, sleep: sleepContext, status: Status{State: StateChecking}}
	for _, opt := range opts {
		opt(s)
	}
	if source == nil || !validSemver(current) {
		s.status = Status{State: StateNone}
	}
	return s
}
