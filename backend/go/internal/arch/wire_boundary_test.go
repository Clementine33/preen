package arch_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const httpapiImportPath = module + "/adapters/httpapi"

type exportedPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	Export     string
	ImportMap  map[string]string
}

func TestHTTPAPIWireTypesStayInsideBoundary(t *testing.T) {
	fset, files, info := loadAndCheckHTTPAPI(t)
	violations := inspectWireBoundary(httpapiImportPath, fset, files, info)
	for _, v := range violations {
		t.Error(v)
	}
}

func TestWireBoundaryRuleRejectsViolations(t *testing.T) {
	const testHTTPAPI = "example.com/test/httpapi"
	fset, files, info := checkSyntheticWirePackage(t, testHTTPAPI)

	violations := inspectWireBoundary(testHTTPAPI, fset, files, info)
	joined := strings.Join(violations, "\n")
	wantCount := 14 // 5 writeJSON + 4 decodeJSON* + 1 direct json.Decoder.Decode + 4 sw.Send
	if len(violations) != wantCount {
		t.Fatalf("got %d violations, want %d:\n%s", len(violations), wantCount, joined)
	}
	for _, needle := range []string{
		"writeJSON", "decodeJSON", "decodeJSONAllowEmpty", "SSEWriter.Send", "json.Decoder.Decode",
	} {
		if !strings.Contains(joined, needle) {
			t.Errorf("expected violation mentioning %q in:\n%s", needle, joined)
		}
	}
}

func checkSyntheticWirePackage(t *testing.T, pkgPath string) (*token.FileSet, []*ast.File, *types.Info) {
	t.Helper()
	const testDomain = "example.com/test/domain"

	fset := token.NewFileSet()
	domainFile, err := parser.ParseFile(fset, "domain.go", `package domain
type Entity struct { ID string }
`, 0)
	if err != nil {
		t.Fatalf("parse domain: %v", err)
	}
	domainPkg, err := (&types.Config{}).Check(testDomain, fset, []*ast.File{domainFile}, nil)
	if err != nil {
		t.Fatalf("check domain: %v", err)
	}

	transportFile, err := parser.ParseFile(fset, "transport.go", `package httpapi
import "encoding/json"
import "io"
type SSEWriter struct{}
func (s *SSEWriter) Send(event string, data any) error { return nil }
func writeJSON(w any, status int, value any) {}
func decodeJSON(w any, r io.Reader, dst any) error {
	return json.NewDecoder(r).Decode(dst)
}
func decodeJSONAllowEmpty(w any, r io.Reader, dst any) error {
	return json.NewDecoder(r).Decode(dst)
}
`, 0)
	if err != nil {
		t.Fatalf("parse transport: %v", err)
	}

	badHandlers, err := parser.ParseFile(fset, "handlers_bad.go", `package httpapi
import (
	"encoding/json"
	"io"
	"example.com/test/domain"
)
type wrappedDomain struct {
	Entity domain.Entity `+"`json:\"entity\"`"+`
}
type aliasDomain = domain.Entity
type goodDTO struct {
	Name string `+"`json:\"name\"`"+`
	Raw  json.RawMessage `+"`json:\"raw\"`"+`
}
func badHandlers(sw *SSEWriter, r io.Reader) {
	writeJSON(nil, 200, struct{ OK bool }{OK: true})
	writeJSON(nil, 200, map[string]any{"ok": true})
	writeJSON(nil, 200, domain.Entity{})
	writeJSON(nil, 200, wrappedDomain{})
	writeJSON(nil, 200, aliasDomain{})
	writeJSON(nil, 200, goodDTO{})

	var decAnon struct{ Name string }
	_ = decodeJSON(nil, r, &decAnon)
	var decDomain domain.Entity
	_ = decodeJSON(nil, r, &decDomain)
	var decWrapped wrappedDomain
	_ = decodeJSONAllowEmpty(nil, r, &decWrapped)
	var decRaw json.RawMessage
	_ = decodeJSON(nil, r, &decRaw)
	var decGood goodDTO
	_ = decodeJSON(nil, r, &decGood)
	_ = json.NewDecoder(r).Decode(&decGood)

	_ = sw.Send("ping", struct{}{})
	_ = sw.Send("err", map[string]string{"code": "X"})
	_ = sw.Send("dom", domain.Entity{})
	var anyVal any = goodDTO{}
	_ = sw.Send("any", anyVal)
	_ = sw.Send("ok", goodDTO{})
}
`, 0)
	if err != nil {
		t.Fatalf("parse handlers_bad: %v", err)
	}

	stdImporter := importer.ForCompiler(fset, "gc", nil)
	cfg := types.Config{
		Importer: fnImporter(func(path string) (*types.Package, error) {
			if path == testDomain {
				return domainPkg, nil
			}
			return stdImporter.Import(path)
		}),
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	files := []*ast.File{transportFile, badHandlers}
	if _, checkErr := cfg.Check(pkgPath, fset, files, info); checkErr != nil {
		t.Fatalf("check synthetic httpapi: %v", checkErr)
	}
	return fset, files, info
}

type fnImporter func(path string) (*types.Package, error)

func (f fnImporter) Import(path string) (*types.Package, error) { return f(path) }

func loadAndCheckHTTPAPI(t *testing.T) (*token.FileSet, []*ast.File, *types.Info) {
	t.Helper()
	out, err := exec.Command("go", "list", "-export", "-deps", "-json", httpapiImportPath).Output()
	if err != nil {
		t.Fatalf("go list httpapi: %v", err)
	}
	exports := map[string]string{}
	var target exportedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var pkg exportedPackage
		if err = dec.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode go list: %v", err)
		}
		if pkg.Export != "" {
			exports[pkg.ImportPath] = pkg.Export
		}
		if pkg.ImportPath == httpapiImportPath {
			target = pkg
		}
	}
	if target.Dir == "" || len(target.GoFiles) == 0 {
		t.Fatalf("missing httpapi package files in go list output")
	}

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(target.GoFiles))
	for _, name := range target.GoFiles {
		path := filepath.Join(target.Dir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		files = append(files, file)
	}

	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		if mapped, ok := target.ImportMap[path]; ok {
			path = mapped
		}
		exportPath, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(exportPath)
	})

	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	cfg := types.Config{Importer: imp}
	if _, err := cfg.Check(httpapiImportPath, fset, files, info); err != nil {
		t.Fatalf("typecheck httpapi: %v", err)
	}
	return fset, files, info
}

func inspectWireBoundary(pkgPath string, fset *token.FileSet, files []*ast.File, info *types.Info) []string {
	var violations []string
	for _, file := range files {
		fileName := filepath.Base(fset.Position(file.Package).Filename)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn := calledFunc(info, call.Fun)
			if fn == nil || fn.Pkg() == nil {
				return true
			}
			pos := fset.Position(call.Pos())
			switch {
			case fn.Pkg().Path() == pkgPath && fn.Name() == "writeJSON" && len(call.Args) == 3:
				if err := checkWireArg(pkgPath, info.Types[call.Args[2]].Type, false); err != nil {
					violations = append(violations,
						fmt.Sprintf("%s:%d: writeJSON value %v", pos.Filename, pos.Line, err))
				}
			case fn.Pkg().Path() == pkgPath &&
				(fn.Name() == "decodeJSON" || fn.Name() == "decodeJSONAllowEmpty") &&
				len(call.Args) == 3:
				if err := checkWireArg(pkgPath, info.Types[call.Args[2]].Type, true); err != nil {
					violations = append(violations,
						fmt.Sprintf("%s:%d: %s target %v", pos.Filename, pos.Line, fn.Name(), err))
				}
			case fn.Pkg().Path() == pkgPath && fn.Name() == "Send" && isSSEWriterRecv(fn) && len(call.Args) == 2:
				if err := checkWireArg(pkgPath, info.Types[call.Args[1]].Type, false); err != nil {
					violations = append(violations,
						fmt.Sprintf("%s:%d: SSEWriter.Send payload %v", pos.Filename, pos.Line, err))
				}
			case fileName != "transport.go" && isJSONDecoderDecode(fn):
				violations = append(violations,
					fmt.Sprintf("%s:%d: direct json.Decoder.Decode bypasses decodeJSON boundary",
						pos.Filename, pos.Line))
			}
			return true
		})
	}
	return violations
}

func calledFunc(info *types.Info, fun ast.Expr) *types.Func {
	switch expr := fun.(type) {
	case *ast.Ident:
		fn, _ := info.Uses[expr].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[expr]; ok {
			fn, _ := sel.Obj().(*types.Func)
			return fn
		}
		fn, _ := info.Uses[expr.Sel].(*types.Func)
		return fn
	default:
		return nil
	}
}

func isSSEWriterRecv(fn *types.Func) bool {
	recv := fn.Signature().Recv()
	if recv == nil {
		return false
	}
	t := recv.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Name() == "SSEWriter"
}

func isJSONDecoderDecode(fn *types.Func) bool {
	if fn.Pkg().Path() != "encoding/json" || fn.Name() != "Decode" {
		return false
	}
	recv := fn.Signature().Recv()
	if recv == nil {
		return false
	}
	t := recv.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Name() == "Decoder"
}

func checkWireArg(pkgPath string, t types.Type, requirePointer bool) error {
	if t == nil {
		return errors.New("has unknown type")
	}
	if requirePointer {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			return fmt.Errorf("must be a pointer, got %s", t)
		}
		t = ptr.Elem()
	} else {
		for {
			ptr, ok := t.(*types.Pointer)
			if !ok {
				break
			}
			t = ptr.Elem()
		}
	}
	unaliased := types.Unalias(t)
	named, ok := unaliased.(*types.Named)
	if !ok {
		return fmt.Errorf("must be a named type defined in httpapi, got %s", t)
	}
	if named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != pkgPath {
		return fmt.Errorf("must be defined in httpapi, got %s", t)
	}
	return checkWireNested(pkgPath, t, "root", map[types.Type]bool{})
}

func checkWireNested(pkgPath string, t types.Type, path string, seen map[types.Type]bool) error {
	t = types.Unalias(t)
	switch v := t.(type) {
	case *types.Basic:
		return nil
	case *types.Pointer:
		return checkWireNested(pkgPath, v.Elem(), path, seen)
	case *types.Slice:
		return checkWireNested(pkgPath, v.Elem(), path+"[]", seen)
	case *types.Array:
		return checkWireNested(pkgPath, v.Elem(), path+"[]", seen)
	case *types.Map:
		if err := checkWireNested(pkgPath, v.Key(), path+".<key>", seen); err != nil {
			return err
		}
		return checkWireNested(pkgPath, v.Elem(), path+".<val>", seen)
	case *types.Struct:
		for i := range v.NumFields() {
			field := v.Field(i)
			if !field.Exported() && !field.Embedded() {
				continue
			}
			if reflect.StructTag(v.Tag(i)).Get("json") == "-" {
				continue
			}
			if err := checkWireNested(pkgPath, field.Type(), path+"."+field.Name(), seen); err != nil {
				return err
			}
		}
		return nil
	case *types.Named:
		if seen[v] {
			return nil
		}
		seen[v] = true
		pkg := v.Obj().Pkg()
		if pkg == nil {
			return fmt.Errorf("at %s: builtin type %s is not allowed on wire", path, v)
		}
		if isAllowedWireStdType(pkg.Path(), v.Obj().Name()) {
			return nil
		}
		if pkg.Path() != pkgPath {
			return fmt.Errorf("at %s: external type %s must be wrapped or converted in httpapi", path, v)
		}
		return checkWireNested(pkgPath, v.Underlying(), path, seen)
	default:
		return fmt.Errorf("at %s: unsupported wire type %s", path, t)
	}
}

func isAllowedWireStdType(pkgPath, name string) bool {
	return (pkgPath == "time" && name == "Time") ||
		(pkgPath == "encoding/json" && name == "RawMessage") ||
		(pkgPath == "encoding/json/jsontext" && name == "Value")
}
