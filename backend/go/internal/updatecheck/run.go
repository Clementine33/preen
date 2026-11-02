package updatecheck

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Retry policy: three attempts in all, each bounded, with the wait doubling
// from two seconds. Only transient failures are retried.
const (
	maxAttempts    = 3
	attemptTimeout = 10 * time.Second
	firstBackoff   = 2 * time.Second
)

// transientError marks a failure worth another attempt.
type transientError struct{ err error }

func (e transientError) Error() string { return e.err.Error() }
func (e transientError) Unwrap() error { return e.err }

// Transient marks err as worth another attempt: a network failure, a server
// error or a rate limit. Other errors, such as a missing release or a body
// that cannot be read, end the check at once.
func Transient(err error) error {
	if err == nil {
		return nil
	}
	return transientError{err}
}

// Run performs the one check and returns when it has settled, or when ctx is
// done. A failure is logged once and reads as StateNone.
func (s *Service) Run(ctx context.Context) {
	if s.source == nil || s.Status().State != StateChecking {
		return
	}
	status, err := s.check(ctx)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		slog.Warn("update check failed", "err", err)
	}
	s.settle(status)
}

// check asks the source up to maxAttempts times and compares the answer.
func (s *Service) check(ctx context.Context) (Status, error) {
	backoff := firstBackoff
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var release Release
		release, err = s.attempt(ctx)
		if err == nil {
			return s.compare(release)
		}
		var transient transientError
		if ctx.Err() != nil || !errors.As(err, &transient) || attempt == maxAttempts {
			break
		}
		if sleepErr := s.sleep(ctx, backoff); sleepErr != nil {
			return Status{State: StateNone}, sleepErr
		}
		backoff *= 2
	}
	return Status{State: StateNone}, err
}

func (s *Service) attempt(ctx context.Context) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	return s.source(ctx)
}

func (s *Service) compare(release Release) (Status, error) {
	if !validSemver(release.Tag) {
		return Status{State: StateNone}, fmt.Errorf("release tag %q is not a semantic version", release.Tag)
	}
	if compareSemver(release.Tag, s.current) <= 0 {
		return Status{State: StateNone}, nil
	}
	return Status{State: StateAvailable, LatestVersion: release.Tag, ReleaseURL: release.URL}, nil
}
