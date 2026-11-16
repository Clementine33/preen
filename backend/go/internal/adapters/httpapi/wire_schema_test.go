//nolint:testpackage // the generator reflects the adapter's private DTO declarations
package httpapi

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/invopop/jsonschema"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
	"github.com/Clementine33/preen/backend/internal/dirrename"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// TestGenerateWireSchema runs only through go generate. Ordinary tests consume
// checked-in schemas, so a changed DTO cannot silently update their expectations.
func TestGenerateWireSchema(t *testing.T) {
	if os.Getenv("PREEN_GENERATE_SCHEMA") != "1" {
		t.Skip("go generate only")
	}
	TestWireTaskRegistry(t)
	dir := os.Getenv("PREEN_SCHEMA_DIR")
	if dir == "" {
		dir = "../../../../../docs/api/schema"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWireContracts(t, dir)
	roots := wireRoots()
	roots = append(roots, taskWireRoots()...)
	for _, value := range roots {
		typ := reflect.TypeOf(value)
		schema := reflectWireSchema(t, typ)
		if typ == reflect.TypeFor[executionViewResponse]() {
			writeExecutionViewSchemas(t, dir, schema)
		}
		name := schemaName(typ)
		if hasTaskWireFields(schema) {
			var variants []any
			for _, task := range wireTasks() {
				bound := cloneWireMap(schema)
				bindTaskWireSchema(bound, task)
				boundName := task.kind + "_v1_" + name
				bound["$id"] = boundName + ".json"
				bound["title"] = boundName
				writeWireSchema(t, dir, boundName, bound)
				variants = append(variants, schemaRef(boundName))
			}
			schema = map[string]any{"anyOf": variants, "title": name, "$id": name + ".json"}
		}
		writeWireSchema(t, dir, name, schema)
		if name == "conversion_DraftDoc" || name == "conversion_CommonSettings" || name == "reconcile_Policy" {
			legacy := reflectWireSchema(t, typ)
			defs, _ := legacy["$defs"].(map[string]any)
			def, _ := defs[name].(map[string]any)
			required, _ := def["required"].([]any)
			keep := []any{"classifier_tags"}
			for _, field := range required {
				if field != "no_effect_tags" && field != "other_tags" {
					keep = append(keep, field)
				}
			}
			def["required"] = keep
			legacy["$id"] = name + "_legacy.json"
			legacy["title"] = name + "_legacy"
			writeWireSchema(t, dir, name+"_legacy", legacy)
			writeWireSchema(
				t,
				dir,
				name+"_stored",
				map[string]any{
					"$id":   name + "_stored.json",
					"title": name + "_stored",
					"anyOf": []any{schemaRef(name), schemaRef(name + "_legacy")},
				},
			)
		}
		if isTaskInput(typ) {
			input := cloneWireMap(schema)
			optionalRequestFields(input)
			input["$id"] = name + "_request.json"
			input["title"] = name + "_request"
			writeWireSchema(t, dir, name+"_request", input)
		}
	}
	writeWireSchema(
		t,
		dir,
		"errorWithDetails",
		map[string]any{"allOf": []any{schemaRef("errorResponse"), map[string]any{"required": []string{"details"}}}},
	)
	writeWireSchema(t, dir, "sseError", schemaRef("errorResponse"))
}

// writeWireContracts emits the route bindings plus each Task's draft request
// schema, which the web client cannot derive from the generic/bound schema pairs.
func writeWireContracts(t *testing.T, dir string) {
	t.Helper()
	var contracts map[string]any
	if err := json.Unmarshal([]byte(wireContracts), &contracts); err != nil {
		t.Fatal(err)
	}
	draftRequests := map[string]string{}
	for _, task := range wireTasks() {
		draftRequests[task.kind] = task.draftRequest
	}
	contracts["task_draft_requests"] = draftRequests
	raw, err := json.MarshalIndent(contracts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contracts.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func taskWireRoots() []any {
	return []any{
		conversion.CommonSettings{},
		conversion.DraftDoc{},
		conversion.ExecutionOptions{},
		conversion.FrozenMemberConclusion{},
		reconcile.Policy{},
		reconcile.StepSummary{},
		reconcile.ComponentOutcome{},
		reconcile.VariantDecision{},
		reconcile.FileDecision{},
		dirrename.Common{},
		dirrename.Draft{},
		dirrename.Outcome{},
		dirrename.Result{},
		refresh.Common{},
		refresh.Draft{},
		refresh.Outcome{},
		refresh.WorkResult{},
		refresh.PlanSummary{},
		workset.PlanSummary{},
	}
}

func schemaName(typ reflect.Type) string {
	if typ.Name() == "" {
		return ""
	}
	if typ.PkgPath() == reflect.TypeFor[errorResponse]().PkgPath() {
		return typ.Name()
	}
	return path.Base(typ.PkgPath()) + "_" + typ.Name()
}

func reflectWireSchema(t *testing.T, typ reflect.Type) map[string]any {
	t.Helper()
	r := jsonschema.Reflector{Anonymous: true, Namer: schemaName}
	r.Mapper = func(typ reflect.Type) *jsonschema.Schema {
		switch typ {
		case reflect.TypeFor[nullableInt]():
			return &jsonschema.Schema{OneOf: []*jsonschema.Schema{{Type: "integer"}, {Type: "null"}}}
		case reflect.TypeFor[nullableString]():
			return &jsonschema.Schema{OneOf: []*jsonschema.Schema{{Type: "string"}, {Type: "null"}}}
		case reflect.TypeFor[json.RawMessage]():
			return &jsonschema.Schema{}
		}
		return nil
	}
	raw, err := json.Marshal(r.ReflectFromType(typ))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	defs, _ := out["$defs"].(map[string]any)
	types := map[string]reflect.Type{}
	collectWireTypes(typ, types)
	for name, v := range defs {
		def, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if dt, exists := types[name]; exists {
			fixWireFields(def, dt)
		}
		overrideWireShape(name, def)
	}
	if def, ok := defs["taskPayloadResponse"].(map[string]any); ok {
		switch typ.Name() {
		case "revisionResponse":
			removeWireProperty(def, "components")
		case "memberReviewResponse":
			required, _ := def["required"].([]any)
			def["required"] = append(required, "components")
		}
	}
	out["$id"] = schemaName(typ) + ".json"
	out["title"] = schemaName(typ)
	if strings.HasSuffix(typ.Name(), "Request") {
		optionalRequestFields(out)
	}
	return out
}

func collectWireTypes(typ reflect.Type, out map[string]reflect.Type) {
	// Only composite types contain nested wire fields.
	//exhaustive:ignore
	switch typ.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		collectWireTypes(typ.Elem(), out)
	case reflect.Struct:
		name := schemaName(typ)
		if _, seen := out[name]; seen {
			return
		}
		out[name] = typ
		for f := range typ.Fields() {
			if f.IsExported() || f.Anonymous {
				collectWireTypes(f.Type, out)
			}
		}
	}
}

func fixWireFields(def map[string]any, typ reflect.Type) {
	props, ok := def["properties"].(map[string]any)
	if !ok {
		return
	}
	// Adapter DTOs convert nil slices to []; Task payload types describe stored
	// records that may still hold null (docs/api/common.md#wire-schemas).
	strictArrays := typ.PkgPath() == reflect.TypeFor[errorResponse]().PkgPath()
	for field := range typ.Fields() {
		tag := strings.Split(field.Tag.Get("json"), ",")
		name := tag[0]
		if field.Anonymous && name == "" {
			fixWireFields(def, field.Type)
			continue
		}
		if name == "" {
			name = field.Name
		}
		prop, exists := props[name]
		if !exists {
			continue
		}
		omitted := strings.Contains(field.Tag.Get("json"), ",omitempty") ||
			strings.Contains(field.Tag.Get("json"), ",omitzero")
		props[name] = wireNullable(prop, field.Type, omitted, strictArrays)
	}
}

func wireNullable(prop any, typ reflect.Type, omitted, strictArrays bool) any {
	// Only composite types contain nested wire fields.
	//exhaustive:ignore
	switch typ.Kind() {
	case reflect.Pointer:
		prop = wireNullable(prop, typ.Elem(), false, strictArrays)
		if !omitted {
			return map[string]any{"anyOf": []any{prop, map[string]any{"type": "null"}}}
		}
	case reflect.Slice, reflect.Map:
		if strictArrays && typ.Kind() == reflect.Slice {
			return prop
		}
		if !omitted && typ != reflect.TypeFor[json.RawMessage]() {
			return map[string]any{"anyOf": []any{prop, map[string]any{"type": "null"}}}
		}
	}
	return prop
}

func optionalRequestFields(v any) {
	switch v := v.(type) {
	case map[string]any:
		delete(v, "required")
		for _, child := range v {
			optionalRequestFields(child)
		}
	case []any:
		for _, child := range v {
			optionalRequestFields(child)
		}
	}
}

func schemaRef(name string) map[string]any { return map[string]any{"$ref": name + ".json"} }

func writeWireSchema(t *testing.T, dir, name string, schema map[string]any) {
	t.Helper()
	if _, ok := schema["$schema"]; !ok {
		schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	}
	raw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, name+".json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
