package arch_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/Clementine33/preen/backend/internal"

// goPackage describes production edges only: tests may use real adapters.
type goPackage struct {
	ImportPath string
	Imports    []string
	Deps       []string
}

type rule struct {
	name   string
	pkg    string
	banned []string
	direct bool
	why    string
}

// Pure cores opt in by name; other business packages may coordinate I/O through
// ports. os is judged directly because innocent stdlib code such as fmt uses it.
var pureCores = []string{"/conversion/reconcile", "/librarypath"}

func TestDependenciesFollowTheLayeringRules(t *testing.T) {
	packages := productionPackages(t)
	rules := architectureRules(packages)
	for _, tc := range rules {
		t.Run(tc.pkg+"/"+tc.name, func(t *testing.T) {
			if tc.why == "" {
				t.Fatal("an architecture rule must explain its purpose")
			}
			pkg, ok := packages[tc.pkg]
			if !ok {
				t.Fatalf("architecture rule references missing package %s", tc.pkg)
			}
			edges := pkg.Deps
			if tc.direct {
				edges = pkg.Imports
			}
			for _, banned := range tc.banned {
				if hit := match(edges, banned); hit != "" {
					t.Fatalf("%s: %s imports %s: %s", tc.name, tc.pkg, hit, tc.why)
				}
			}
		})
	}
}

func architectureRules(packages map[string]goPackage) []rule {
	var rules []rule
	// Default inclusion prevents new business packages escaping the boundary by
	// omission from a registry. Only assembly, adapters and the guard are exempt.
	for path := range packages {
		if (path != module && !strings.HasPrefix(path, module+"/")) ||
			match([]string{path}, module+"/adapters") != "" ||
			match([]string{path}, module+"/app") != "" ||
			match([]string{path}, module+"/arch") != "" {
			continue
		}
		rules = append(rules, rule{
			name: "business uses consumer ports", pkg: path,
			banned: []string{module + "/adapters", module + "/app"},
			why:    "business rules remain reusable without storage, transport or process assembly",
		})
	}
	for _, core := range pureCores {
		rules = append(rules, rule{
			name: "pure core has no I/O", pkg: module + core, direct: true,
			banned: []string{"os", "net", "os/exec", "database/sql", "io/fs"},
			why:    "path semantics and reconciliation must work from supplied facts on every host",
		}, rule{
			name: "pure core has no I/O dependency", pkg: module + core,
			banned: []string{"os/exec", "database/sql", "net"},
			why:    "a core cannot delegate process, database or network access to another package",
		})
	}
	return append(rules,
		rule{
			name: "transport uses business entries", pkg: module + "/adapters/httpapi",
			banned: []string{module + "/adapters/sqlite"},
			why:    "HTTP routes must preserve the validation and transaction boundaries of their use cases",
		},
		rule{
			name: "generic lifecycle is independent of task kinds", pkg: module + "/workset",
			banned: []string{module + "/conversion", module + "/dirrename", module + "/works/refresh",
				module + "/works/coverdelivery"},
			why: "tasks implement workset seams so adding a kind does not change the generic lifecycle",
		},
		rule{
			name: "catalog is independent of child workflows", pkg: module + "/works",
			banned: []string{module + "/works/refresh", module + "/works/coverdelivery"},
			why:    "nested directories state ownership; the base catalog remains reusable by its workflows",
		},
	)
}

func productionPackages(t *testing.T) map[string]goPackage {
	t.Helper()
	out, err := exec.Command("go", "list", "-json", module+"/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	packages := map[string]goPackage{}
	decoder := json.NewDecoder(bytes.NewReader(out))
	for {
		var pkg goPackage
		if err = decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("go list output: %v", err)
		}
		packages[pkg.ImportPath] = pkg
	}
	return packages
}

func match(edges []string, banned string) string {
	for _, edge := range edges {
		if edge == banned || strings.HasPrefix(edge, banned+"/") {
			return edge
		}
	}
	return ""
}
