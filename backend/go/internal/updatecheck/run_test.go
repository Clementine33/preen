package updatecheck_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/updatecheck"
)

// script answers each attempt in order and records how often it was asked and
// how long the service waited between attempts.
type script struct {
	answers []func() (updatecheck.Release, error)
	calls   int
	waits   []time.Duration
}

func (s *script) source(context.Context) (updatecheck.Release, error) {
	s.calls++
	answer := s.answers[min(s.calls, len(s.answers))-1]
	return answer()
}

func (s *script) sleep(_ context.Context, d time.Duration) error {
	s.waits = append(s.waits, d)
	return nil
}

func release(tag string) func() (updatecheck.Release, error) {
	return func() (updatecheck.Release, error) {
		return updatecheck.Release{Tag: tag, URL: "https://example.test/" + tag}, nil
	}
}

func failure(err error) func() (updatecheck.Release, error) {
	return func() (updatecheck.Release, error) { return updatecheck.Release{}, err }
}

func run(current string, s *script) *updatecheck.Service {
	svc := updatecheck.New(current, s.source, updatecheck.WithSleep(s.sleep))
	svc.Run(context.Background())
	return svc
}

func TestStatusComparesTheLatestTagWithTheRunningVersion(t *testing.T) {
	for _, c := range []struct {
		name, current, tag, want string
	}{
		{"newer release", "0.2.0", "v0.3.0", updatecheck.StateAvailable},
		{"current has a v", "v0.2.0", "v0.2.1", updatecheck.StateAvailable},
		{"equal", "0.2.0", "v0.2.0", updatecheck.StateNone},
		{"older release", "0.3.0", "v0.2.0", updatecheck.StateNone},
		{"stable beats its rc", "0.2.0-rc.2", "v0.2.0", updatecheck.StateAvailable},
		{"rc does not beat its stable", "0.2.0", "v0.2.0-rc.2", updatecheck.StateNone},
		{"later rc", "0.2.0-rc.2", "v0.2.0-rc.10", updatecheck.StateAvailable},
		{"build metadata ignored", "0.2.0+abc", "v0.2.0", updatecheck.StateNone},
		{"numeric not lexical", "0.9.0", "v0.10.0", updatecheck.StateAvailable},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc := run(c.current, &script{answers: []func() (updatecheck.Release, error){release(c.tag)}})
			got := svc.Status()
			if got.State != c.want {
				t.Fatalf("state = %q, want %q", got.State, c.want)
			}
			if c.want == updatecheck.StateAvailable && (got.LatestVersion != c.tag || got.ReleaseURL != "https://example.test/"+c.tag) {
				t.Fatalf("status = %+v", got)
			}
			if c.want != updatecheck.StateAvailable && (got.LatestVersion != "" || got.ReleaseURL != "") {
				t.Fatalf("release fields set on %q: %+v", got.State, got)
			}
		})
	}
}

func TestStatusIsCheckingUntilTheCheckSettles(t *testing.T) {
	svc := updatecheck.New("0.2.0", func(context.Context) (updatecheck.Release, error) {
		return updatecheck.Release{}, nil
	})
	if got := svc.Status().State; got != updatecheck.StateChecking {
		t.Fatalf("state before Run = %q", got)
	}
}

func TestSkippedChecksReportNoneWithoutAsking(t *testing.T) {
	for _, current := range []string{"dev", "", "latest", "0.2.0.1"} {
		s := &script{answers: []func() (updatecheck.Release, error){release("v9.9.9")}}
		svc := updatecheck.New(current, s.source, updatecheck.WithSleep(s.sleep))
		if got := svc.Status().State; got != updatecheck.StateNone {
			t.Errorf("%q: state before Run = %q", current, got)
		}
		svc.Run(context.Background())
		if s.calls != 0 || svc.Status().State != updatecheck.StateNone {
			t.Errorf("%q: calls = %d, state = %q", current, s.calls, svc.Status().State)
		}
	}
	svc := updatecheck.New("0.2.0", nil)
	svc.Run(context.Background())
	if got := svc.Status().State; got != updatecheck.StateNone {
		t.Fatalf("nil source: state = %q", got)
	}
}

func TestTransientFailuresRetryThreeTimesWithBackoff(t *testing.T) {
	boom := updatecheck.Transient(errors.New("status 503"))

	s := &script{answers: []func() (updatecheck.Release, error){failure(boom)}}
	svc := run("0.2.0", s)
	if s.calls != 3 {
		t.Fatalf("attempts = %d, want 3", s.calls)
	}
	if len(s.waits) != 2 || s.waits[0] != 2*time.Second || s.waits[1] != 4*time.Second {
		t.Fatalf("waits = %v, want [2s 4s]", s.waits)
	}
	if got := svc.Status(); got.State != updatecheck.StateNone {
		t.Fatalf("status after giving up = %+v", got)
	}

	s = &script{answers: []func() (updatecheck.Release, error){failure(boom), failure(boom), release("v0.3.0")}}
	if got := run("0.2.0", s).Status().State; s.calls != 3 || got != updatecheck.StateAvailable {
		t.Fatalf("recovery: attempts = %d, state = %q", s.calls, got)
	}
}

func TestPermanentFailuresAreNotRetried(t *testing.T) {
	for name, answer := range map[string]func() (updatecheck.Release, error){
		"not found":   failure(errors.New("status 404")),
		"bad tag":     release("nightly"),
		"unparseable": failure(errors.New("decode release")),
	} {
		s := &script{answers: []func() (updatecheck.Release, error){answer}}
		svc := run("0.2.0", s)
		if s.calls != 1 || len(s.waits) != 0 || svc.Status().State != updatecheck.StateNone {
			t.Errorf("%s: attempts = %d, waits = %v, state = %q", name, s.calls, s.waits, svc.Status().State)
		}
	}
}

func TestCancellationStopsTheCheckDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	svc := updatecheck.New("0.2.0", func(context.Context) (updatecheck.Release, error) {
		calls++
		return updatecheck.Release{}, updatecheck.Transient(errors.New("network down"))
	}, updatecheck.WithSleep(func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}))
	svc.Run(ctx)
	if calls != 1 {
		t.Fatalf("attempts after cancel = %d, want 1", calls)
	}
	if got := svc.Status().State; got != updatecheck.StateChecking {
		t.Fatalf("a cancelled check settled as %q", got)
	}
}

func TestEachAttemptIsBoundedByATimeout(t *testing.T) {
	var remaining time.Duration
	svc := updatecheck.New("0.2.0", func(ctx context.Context) (updatecheck.Release, error) {
		// Measured inside the attempt: a start taken before Run lands just under the deadline's own Now.
		if deadline, ok := ctx.Deadline(); ok {
			remaining = time.Until(deadline)
		}
		return updatecheck.Release{Tag: "v0.2.0"}, nil
	})
	svc.Run(context.Background())
	if d := remaining; d <= 0 || d > 10*time.Second {
		t.Fatalf("attempt deadline in %v, want within 10s", d)
	}
}
