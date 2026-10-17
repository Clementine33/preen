package conversion_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/onsei/organizer/backend/internal/conversion"
)

func TestGenerationRefreshWaitsForOtherSessionsAndObservesCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		release := make(chan struct{})
		failed := errors.New("scan failed")
		scan := func(context.Context, []string, string, func()) error {
			if calls.Add(1) == 1 {
				<-release
				return failed
			}
			return nil
		}
		task := conversion.New(nil, nil, nil, nil, nil, nil, conversion.ExecutionIO{Scan: scan})
		first := make(chan error, 1)
		go func() { first <- task.RefreshGenerationInputs(t.Context(), nil, "", nil, nil) }()
		synctest.Wait()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var cancelRequested atomic.Bool
		checkCancel := func() {
			if cancelRequested.Load() {
				cancel()
			}
		}
		second := make(chan error, 1)
		go func() { second <- task.RefreshGenerationInputs(ctx, nil, "", checkCancel, nil) }()
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("another session entered the scan before the first finished")
		}
		cancelRequested.Store(true)
		time.Sleep(200 * time.Millisecond) // Virtual time: cancellation must be checked while waiting.
		synctest.Wait()
		select {
		case err := <-second:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("waiting refresh returned %v", err)
			}
		default:
			t.Fatal("waiting refresh did not observe cancellation")
		}
		close(release)
		if err := <-first; !errors.Is(err, failed) {
			t.Fatalf("scan error was lost: %v", err)
		}
		if err := task.RefreshGenerationInputs(t.Context(), nil, "", nil, nil); err != nil || calls.Load() != 2 {
			t.Fatalf("scan slot was not released after failure: %v, calls=%d", err, calls.Load())
		}
		if err := task.RefreshGenerationInputs(
			ctx,
			nil,
			"",
			nil,
			nil,
		); !errors.Is(err, context.Canceled) ||
			calls.Load() != 2 {
			t.Fatalf("an already canceled refresh entered the scan: %v, calls=%d", err, calls.Load())
		}
	})
}

func TestGenerationRefreshWithoutSelectedScanKeepsFallback(t *testing.T) {
	task := conversion.New(nil, nil, nil, nil, nil, nil)
	failed := errors.New("refresh failed")
	err := task.RefreshGenerationInputs(t.Context(), nil, "", nil, func() error { return failed })
	if !errors.Is(err, failed) {
		t.Fatalf("fallback returned %v", err)
	}
}
