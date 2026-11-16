//nolint:testpackage // schema bindings describe private adapter envelope fields
package httpapi

import (
	"reflect"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/dirrename"
	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

type wireTask struct{ kind, draft, settings, policy, summary, component, conclusion, options, result string }

func wireTasks() []wireTask {
	return []wireTask{
		{
			kind:       "conversion",
			draft:      "conversion_DraftDoc",
			settings:   "conversion_CommonSettings",
			policy:     "reconcile_Policy",
			summary:    "reconcile_StepSummary",
			component:  "reconcile_ComponentOutcome",
			conclusion: "conversion_FrozenMemberConclusion",
			options:    "conversion_ExecutionOptions",
		},
		{
			kind:       "rename",
			draft:      "dirrename_Draft",
			settings:   "dirrename_Common",
			policy:     "dirrename_Common",
			summary:    "workset_PlanSummary",
			component:  "dirrename_Outcome",
			conclusion: "dirrename_Outcome",
			result:     "dirrename_Result",
		},
		{
			kind:       "metadata_refresh",
			draft:      "refresh_Draft",
			settings:   "refresh_Common",
			policy:     "refresh_Common",
			summary:    "refresh_PlanSummary",
			component:  "refresh_Outcome",
			conclusion: "refresh_Outcome",
			options:    "refresh_Common",
			result:     "refresh_WorkResult",
		},
	}
}

func isTaskInput(typ reflect.Type) bool {
	switch typ {
	case reflect.TypeFor[conversion.CommonSettings](),
		reflect.TypeFor[conversion.DraftDoc](),
		reflect.TypeFor[dirrename.Common](),
		reflect.TypeFor[dirrename.Draft](),
		reflect.TypeFor[refresh.Common](),
		reflect.TypeFor[refresh.Draft]():
		return true
	}
	return false
}

func hasTaskWireFields(schema map[string]any) bool {
	defs, _ := schema["$defs"].(map[string]any)
	for name := range defs {
		if isTaskWireDefinition(name) {
			return true
		}
	}
	return false
}

func isTaskWireDefinition(name string) bool {
	switch name {
	case "taskDraftEnvelope",
		"taskEnvelopeResponse",
		"taskPayloadResponse",
		"taskSettingsRequest",
		"taskSettingsResponse",
		"revisionMemberResponse",
		"memberConclusionResponse",
		"executionViewResponse",
		"executionComponentResponse",
		"executionProgressResponse":
		return true
	}
	return false
}

func bindTaskWireSchema(schema map[string]any, task wireTask) {
	defs, _ := schema["$defs"].(map[string]any)
	for name, value := range defs {
		def, ok := value.(map[string]any)
		if !ok {
			continue
		}
		bindTaskWireDefinition(name, def, task)
	}
}

func bindTaskWireDefinition(name string, def map[string]any, task wireTask) {
	// Execution's custom marshaler has three independently closed shapes.
	if branches, ok := def["oneOf"].([]any); ok && name == "executionViewResponse" {
		for _, branch := range branches {
			if b, ok := branch.(map[string]any); ok {
				bindTaskWireDefinition(name, b, task)
			}
		}
		return
	}
	props, _ := def["properties"].(map[string]any)
	if props == nil {
		return
	}
	switch name {
	case "taskDraftEnvelope", "taskEnvelopeResponse":
		props["kind"] = map[string]any{"const": task.kind, "type": "string"}
		props["schema_version"] = map[string]any{"const": 1, "type": "integer"}
		if name == "taskDraftEnvelope" {
			props["payload"] = schemaRef(task.draft)
		}
	case "taskSettingsResponse":
		props["operation_type"] = map[string]any{"const": task.kind, "type": "string"}
		props["settings"] = schemaRef(task.settings)
	case "taskSettingsRequest":
		props["settings"] = schemaRef(task.settings + "_request")
	case "taskPayloadResponse":
		props["policy"] = nullableWireRef(task.policy)
		props["summary"] = nullableWireRef(task.summary)
		props["classifier"] = schemaRef("taskClassifierSnapshot")
		if _, present := props["components"]; present {
			props["components"] = map[string]any{"type": "array", "items": schemaRef(task.component)}
		}
	case "revisionMemberResponse":
		props["effective"] = nullableWireRef(task.policy)
	case "memberConclusionResponse":
		props["task"] = schemaRef(task.conclusion)
	case "executionProgressResponse", "executionViewResponse":
		if task.options == "" {
			delete(props, "options")
		} else {
			props["options"] = schemaRef(task.options)
		}
		if name == "executionViewResponse" {
			props["operation_type"] = map[string]any{"const": task.kind, "type": "string"}
		}
	case "executionComponentResponse":
		props["kept_decisions"] = map[string]any{"type": "array", "maxItems": 0}
		switch task.kind {
		case "conversion":
			delete(props, "task_result")
			props["kept_decisions"] = map[string]any{"type": "array", "items": schemaRef("reconcile_FileDecision")}
		case "metadata_refresh":
			props["task_result"] = map[string]any{
				"oneOf": []any{schemaRef("refresh_Outcome"), schemaRef("refresh_WorkResult")},
			}
			def["allOf"] = []any{
				taskResultStage("skipped", "refresh_Outcome"), taskResultStage("metadata", "refresh_WorkResult"),
			}
		default:
			props["task_result"] = schemaRef(task.result)
		}
	}
}

func taskResultStage(stage, result string) map[string]any {
	return map[string]any{
		"if": map[string]any{
			"required":   []string{"stage"},
			"properties": map[string]any{"stage": map[string]any{"const": stage}},
		},
		"then": map[string]any{"properties": map[string]any{"task_result": schemaRef(result)}},
	}
}

func nullableWireRef(name string) map[string]any {
	return map[string]any{"anyOf": []any{schemaRef(name), map[string]any{"type": "null"}}}
}
