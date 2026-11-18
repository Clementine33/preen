package scraper_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/scraper"
	"github.com/Clementine33/preen/backend/internal/works"
)

func limited(retryAfter string) *http.Response {
	r := response(http.StatusTooManyRequests, `{}`)
	r.Header.Set("Retry-After", retryAfter)
	return r
}

func dynamic(client *scraper.Client, ctx context.Context) error {
	_, err := client.FetchDynamic(ctx, []works.WorkCode{"RJ00000001"}, "ja-jp")
	return err
}

func TestRateLimitedRequestWaitsOutTheWindowAndResendsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return limited("290"), nil
			}
			return response(http.StatusOK, `{}`), nil
		})})
		start := time.Now()
		if err := dynamic(client, context.Background()); err != nil {
			t.Fatalf("request after the window failed: %v", err)
		}
		if calls.Load() != 2 || time.Since(start) < 290*time.Second {
			t.Fatalf("got %d requests after %s, want 2 after 290s", calls.Load(), time.Since(start))
		}
	})
}

func TestRepeatedRateLimitIsNotResentAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return limited("30"), nil
		})})
		if err := dynamic(client, context.Background()); err == nil || calls.Load() != 2 {
			t.Fatalf("got %d requests, err %v; want 2 and an error", calls.Load(), err)
		}
	})
}

func TestRateLimitBeyondTheBudgetFailsFastWithTheRemainingTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return limited("900"), nil
		})})
		start := time.Now()
		if err := dynamic(client, context.Background()); err == nil || !strings.Contains(err.Error(), "15m0s left") {
			t.Fatalf("over-budget error of the rate-limited call: %v", err)
		}
		if err := dynamic(client, context.Background()); err == nil || !strings.Contains(err.Error(), "15m0s left") {
			t.Fatalf("over-budget error of a later call: %v", err)
		}
		if calls.Load() != 1 || time.Since(start) > time.Second {
			t.Fatalf("got %d requests after %s, want 1 without waiting", calls.Load(), time.Since(start))
		}
	})
}

func TestQueuedRequestsResumeAtTheStartIntervalAfterTheWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var starts []time.Time
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			mu.Lock()
			defer mu.Unlock()
			starts = append(starts, time.Now())
			if len(starts) == 1 {
				return limited("60"), nil
			}
			return response(http.StatusOK, `{}`), nil
		})})
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if err := dynamic(client, context.Background()); err != nil {
					t.Errorf("queued request failed: %v", err)
				}
			})
		}
		wg.Wait()
		if len(starts) != 4 {
			t.Fatalf("got %d requests, want 4", len(starts))
		}
		for i := 2; i < len(starts); i++ {
			if gap := starts[i].Sub(starts[i-1]); gap < 100*time.Millisecond {
				t.Fatalf("resumed requests %d apart, want the start interval", gap)
			}
		}
		if starts[1].Sub(starts[0]) < 60*time.Second {
			t.Fatal("a queued request ignored the throttle window")
		}
	})
}

func TestCancelledWaitSendsNothingMore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return limited("290"), nil
		})})
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(10*time.Second, cancel)
		if err := dynamic(client, ctx); err == nil {
			t.Fatal("cancelled wait succeeded")
		}
		time.Sleep(time.Hour)
		if calls.Load() != 1 {
			t.Fatalf("got %d requests, want 1", calls.Load())
		}
	})
}

func TestResentRateLimitBeyondTheBudgetNamesTheTimeLeft(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return limited("60"), nil
			}
			return limited("900"), nil
		})})
		if err := dynamic(client, context.Background()); err == nil || !strings.Contains(err.Error(), "15m0s left") {
			t.Fatalf("error of the resent request: %v", err)
		}
		if calls.Load() != 2 {
			t.Fatalf("got %d requests, want 2", calls.Load())
		}
	})
}

func TestThrottleBudgetSpansTheWholeRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return limited("400"), nil
			}
			return limited("300"), nil
		})})
		// Open a 400s window without the waiter being the request that hit it.
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(time.Second, cancel)
		_ = dynamic(client, ctx)
		start := time.Now()
		err := dynamic(client, context.Background())
		if err == nil || !strings.Contains(err.Error(), "5m0s left") {
			t.Fatalf("over-budget error after waiting: %v", err)
		}
		if calls.Load() != 2 || time.Since(start) > 400*time.Second {
			t.Fatalf("got %d requests after %s, want 2 right after the first window", calls.Load(), time.Since(start))
		}
	})
}

func TestChallengeWakesAThrottleWaiterAndKeepsTheWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		arrived, challenge := make(chan struct{}), make(chan struct{})
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				<-arrived // the challenged request is in flight before this 429 lands
				return limited("290"), nil
			case 2:
				close(arrived)
				<-challenge
				r := response(http.StatusForbidden, `{}`)
				r.Header.Set("Cf-Mitigated", "challenge")
				return r, nil
			}
			return response(http.StatusOK, `{}`), nil
		})})
		start := time.Now()
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() {
				if err := dynamic(client, context.Background()); err == nil {
					t.Error("request during a challenge succeeded")
				}
			})
		}
		<-arrived
		synctest.Wait() // the 429 has landed and its request is waiting out the window
		close(challenge)
		wg.Wait()
		if calls.Load() != 2 || time.Since(start) > time.Second {
			t.Fatalf("got %d requests after %s, want 2 without waiting", calls.Load(), time.Since(start))
		}
		time.Sleep(61 * time.Second)
		if err := dynamic(client, context.Background()); err != nil {
			t.Fatalf("request after the challenge failed: %v", err)
		}
		if calls.Load() != 3 || time.Since(start) < 290*time.Second {
			t.Fatalf("got %d requests after %s, want the 290s window kept", calls.Load(), time.Since(start))
		}
	})
}
