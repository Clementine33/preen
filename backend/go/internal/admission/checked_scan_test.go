package admission_test

import (
	"errors"
	"testing"

	"github.com/onsei/organizer/backend/internal/admission"
)

func TestCheckedScanAndExclusiveEnqueueAreAtomicInBothOrders(t *testing.T) {
	t.Run("refresh first", func(t *testing.T) {
		gate := admission.NewGate(nil)
		checking, finish := make(chan struct{}), make(chan struct{})
		releaseResult := make(chan func())
		go func() {
			release, err := gate.BeginScanChecked(func() error { close(checking); <-finish; return nil })
			if err != nil {
				t.Error(err)
			}
			releaseResult <- release
		}()
		<-checking
		enqueueResult := make(chan error)
		go func() {
			enqueueResult <- gate.EnqueueExclusive(func() error { t.Error("rename entered during refresh"); return nil })
		}()
		close(finish)
		release := <-releaseResult
		if err := <-enqueueResult; !admission.IsBusy(err) {
			t.Fatalf("enqueue: %v", err)
		}
		release()
		if err := gate.EnqueueExclusive(func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rename first", func(t *testing.T) {
		gate := admission.NewGate(nil)
		writing, finish := make(chan struct{}), make(chan struct{})
		enqueued := make(chan error)
		active := false
		busy := errors.New("rename is queued")
		go func() {
			enqueued <- gate.EnqueueExclusive(func() error { close(writing); <-finish; active = true; return nil })
		}()
		<-writing
		scanned := make(chan error)
		go func() {
			release, err := gate.BeginScanChecked(func() error {
				if active {
					return busy
				}
				return nil
			})
			if release != nil {
				release()
			}
			scanned <- err
		}()
		close(finish)
		if err := <-enqueued; err != nil {
			t.Fatal(err)
		}
		if err := <-scanned; !errors.Is(err, busy) {
			t.Fatalf("refresh: %v", err)
		}
		// A refused check must not leave a phantom scan registration.
		if err := gate.EnqueueExclusive(func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	})
}
