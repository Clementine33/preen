package conversion_test

import (
	"encoding/json"
	"testing"

	"github.com/Clementine33/preen/backend/internal/conversion"
)

func TestMemberConclusion(t *testing.T) {
	tests := []struct {
		name       string
		components []json.RawMessage
		status     string
	}{
		{"no applicable component", nil, "not_applicable"},
		{"unchanged", []json.RawMessage{json.RawMessage(`{"partition":"matched","status":"ok"}`)}, "unchanged"},
		{"changed", []json.RawMessage{json.RawMessage(`{"partition":"matched","operations":[{}]}`)}, "changed"},
		{
			"one unmet component",
			[]json.RawMessage{
				json.RawMessage(
					`{"partition":"matched","variant_decisions":[{"decisions":[{"reason_code":"UNMET_TARGET"}]}]}`,
				),
			},
			"unmet",
		},
		{
			"same partition unmet and unchanged",
			[]json.RawMessage{
				json.RawMessage(
					`{"partition":"matched","variant_decisions":[{"decisions":[{"reason_code":"UNMET_TARGET"}]}]}`,
				),
				json.RawMessage(`{"partition":"matched","status":"ok"}`),
			},
			"partially_unmet",
		},
		{
			"unmet before changes",
			[]json.RawMessage{
				json.RawMessage(
					`{"partition":"matched","operations":[{}],"variant_decisions":[{"decisions":[{"reason_code":"UNMET_TARGET"}]}]}`,
				),
			},
			"unmet",
		},
		{
			"source ambiguity before unmet",
			[]json.RawMessage{
				json.RawMessage(
					`{"partition":"matched","status":"blocked","reason_code":"SOURCE_AMBIGUOUS","variant_decisions":[{"decisions":[{"reason_code":"UNMET_TARGET"}]}]}`,
				),
			},
			"source_ambiguous",
		},
		{
			"blocking before source ambiguity",
			[]json.RawMessage{
				json.RawMessage(`{"partition":"matched","status":"blocked","reason_code":"SOURCE_AMBIGUOUS"}`),
				json.RawMessage(`{"partition":"unmatched","status":"blocked","reason_code":"SOURCE_MISSING"}`),
			},
			"blocked",
		},
	}
	task := &conversion.Task{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := task.MemberConclusion(tt.components)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got.Status != tt.status {
				t.Fatalf("status = %q, want %q (%s)", got.Status, tt.status, raw)
			}
		})
	}
	t.Run("independent partition facts", func(t *testing.T) {
		raw, err := task.MemberConclusion(
			[]json.RawMessage{
				json.RawMessage(
					`{"partition":"matched","status":"blocked","operations":[{}],"variant_decisions":[{"decisions":[{"reason_code":"UNMET_TARGET"}]}]}`,
				),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		for side, expected := range map[string]string{"matched": `{"applicable":true,"blocked":true,"unmet":true,"changes":true}`, "unmatched": `{"applicable":false,"blocked":false,"unmet":false,"changes":false}`} {
			if string(got[side]) != expected {
				t.Errorf("%s = %s, want %s", side, got[side], expected)
			}
		}
	})
}
