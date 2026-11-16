//nolint:testpackage // serialization overrides refer to private DTO definitions
package httpapi

import "encoding/json"

// overrideWireShape describes only differences introduced by custom JSON methods
// or opaque payload transport. It never changes server encoding or decoding.
func overrideWireShape(name string, def map[string]any) {
	props, _ := def["properties"].(map[string]any)
	switch name {
	case "libraryPatchRequest":
		props["storage_class"] = map[string]any{"type": "string"}
	case "scanRequest":
		props["mode"] = map[string]any{"type": "string"}
	case "collectionWorkMoveRequest":
		props["before_rj"] = map[string]any{"type": []string{"string", "null"}}

	case "workContentNodeResponse":
		clear(def)
		def["oneOf"] = []any{schemaRef("workContentDirNodeJSON"), schemaRef("workContentFileNodeJSON")}
	case "reconcile_ComponentOutcome":
		forceWireArrays(def, "lanes", "variant_decisions", "operations", "projected_inventory", "files")
	case "reconcile_VariantDecision":
		forceWireArrays(def, "decisions")
	case "workContentDirNodeJSON":
		forceWireArrays(def, "children")
	case "executionViewResponse":
		executionWireShape(def)
	}
}

func forceWireArrays(def map[string]any, fields ...string) {
	props, _ := def["properties"].(map[string]any)
	for _, field := range fields {
		prop, _ := props[field].(map[string]any)
		if variants, ok := prop["anyOf"].([]any); ok {
			props[field] = variants[0]
		}
	}
}

func cloneWireMap(v map[string]any) map[string]any {
	b, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func executionWireShape(def map[string]any) {
	full := cloneWireMap(def)
	members := cloneWireMap(def)
	member := cloneWireMap(def)
	removeWireProperty(full, "members")
	removeWireProperty(full, "member")
	removeWireProperty(members, "components")
	removeWireProperty(members, "member")
	membersRequired, _ := members["required"].([]any)
	members["required"] = append(membersRequired, "members")
	removeWireProperty(member, "members")
	memberRequired, _ := member["required"].([]any)
	member["required"] = append(memberRequired, "member")
	clear(def)
	def["oneOf"] = []any{full, members, member}
}

func removeWireProperty(def map[string]any, key string) {
	props, _ := def["properties"].(map[string]any)
	delete(props, key)
	required, _ := def["required"].([]any)
	keep := required[:0]
	for _, item := range required {
		if item != key {
			keep = append(keep, item)
		}
	}
	def["required"] = keep
}
