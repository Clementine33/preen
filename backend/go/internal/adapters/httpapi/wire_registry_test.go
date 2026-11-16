//nolint:testpackage // the generator registry uses private schema bindings
package httpapi

import (
	"testing"

	appconfig "github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/dirrename"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestWireTaskRegistry(t *testing.T) {
	supported := []workset.Task{
		conversion.New(nil, appconfig.NewReader(""), nil, nil, nil, nil), &dirrename.Task{}, &refresh.Task{},
	}
	bindings := map[string]bool{}
	for _, task := range wireTasks() {
		bindings[task.kind] = true
	}
	for _, task := range supported {
		if !bindings[task.Kind()] {
			t.Fatalf("missing schema binding for Task %q", task.Kind())
		}
		_, _, version := task.SeedDraft(nil)
		if version != 1 {
			t.Fatalf("Task %s schema version %d has no registered wire schema", task.Kind(), version)
		}
	}
}
