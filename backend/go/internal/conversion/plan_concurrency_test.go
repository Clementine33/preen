package conversion_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"

	appconfig "github.com/onsei/organizer/backend/internal/adapters/settings"
	"github.com/onsei/organizer/backend/internal/conversion"
	"github.com/onsei/organizer/backend/internal/inventory"
)

type heldInventory struct {
	release <-chan struct{}
	active  atomic.Int32
	peak    atomic.Int32
}

func (h *heldInventory) ObservedAudioEntries(string) ([]inventory.ObservedAudio, error) {
	active := h.active.Add(1)
	for previous := h.peak.Load(); active > previous; previous = h.peak.Load() {
		if h.peak.CompareAndSwap(previous, active) {
			break
		}
	}
	defer h.active.Add(-1)
	<-h.release
	return nil, nil
}
func (*heldInventory) RootExistsInInventory(string) (bool, error)                { return true, nil }
func (*heldInventory) UpdateEntryBitrates([]inventory.BitrateUpdate, bool) error { return nil }

func TestPlanBoundsWorkConcurrencyAndKeepsRequestOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		inv := &heldInventory{release: release}
		policy := balancedPolicy()
		input := conversion.Input{Policy: policy}
		for index := range 20 {
			input.Roots = append(
				input.Roots,
				conversion.RootInput{Path: fmt.Sprintf("/library/%02d", index), Policy: policy},
			)
		}
		var snapshot *conversion.Snapshot
		var planErr error
		go func() {
			snapshot, planErr = conversion.Plan(context.Background(), inv, appconfig.NewReader(""), nil, input)
		}()
		synctest.Wait()
		active := inv.active.Load()
		close(release)
		synctest.Wait()
		if active != 4 || inv.peak.Load() > 4 {
			t.Fatalf("active/peak = %d/%d", active, inv.peak.Load())
		}
		if planErr != nil {
			t.Fatal(planErr)
		}
		for index, root := range snapshot.Roots {
			if root.Path != input.Roots[index].Path {
				t.Fatalf("request order changed at %d", index)
			}
		}
	})
}
