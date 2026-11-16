//nolint:testpackage // the custom marshaler's branches use private DTOs
package httpapi

import "testing"

func writeExecutionViewSchemas(t *testing.T, dir string, schema map[string]any) {
	t.Helper()
	definitions, _ := schema["$defs"].(map[string]any)
	view, _ := definitions["executionViewResponse"].(map[string]any)
	branches, _ := view["oneOf"].([]any)
	names := []string{"executionFullViewResponse", "executionMembersViewResponse", "executionMemberViewResponse"}
	if len(branches) != len(names) {
		t.Fatal("execution marshaler branches changed without view bindings")
	}
	for i, name := range names {
		var variants []any
		for _, task := range wireTasks() {
			bound := cloneWireMap(schema)
			defs, _ := bound["$defs"].(map[string]any)
			branch, ok := branches[i].(map[string]any)
			if !ok {
				t.Fatal("invalid execution branch")
			}
			defs["executionViewResponse"] = cloneWireMap(branch)
			bindTaskWireSchema(bound, task)
			id := task.kind + "_v1_" + name
			bound["$id"] = id + ".json"
			bound["title"] = id
			writeWireSchema(t, dir, id, bound)
			variants = append(variants, schemaRef(id))
		}
		writeWireSchema(t, dir, name, map[string]any{"$id": name + ".json", "title": name, "anyOf": variants})
	}
}
