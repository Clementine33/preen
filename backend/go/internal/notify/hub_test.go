package notify_test

import (
	"testing"

	"github.com/onsei/organizer/backend/internal/notify"
)

func TestBurstCoalescesToOnePendingSignal(t *testing.T) {
	var h notify.Hub
	ch, release := h.Subscribe(false)
	defer release()
	select {
	case <-ch:
		t.Fatal("unprimed subscriber had a signal")
	default:
	}
	h.Notify()
	h.Notify()
	h.Notify()
	<-ch
	select {
	case <-ch:
		t.Fatal("burst delivered more than one signal")
	default:
	}
	primed, release2 := h.Subscribe(true)
	defer release2()
	<-primed
}
