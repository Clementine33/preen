//nolint:testpackage // the generator registry uses private schema bindings
package httpapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
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
	count := 0
	err := filepath.WalkDir("../..", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv != nil && fn.Name.Name == "Kind" {
				count++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != len(supported) {
		t.Fatalf("%d production Task Kind methods but %d schema registrations", count, len(supported))
	}
}
