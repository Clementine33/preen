package dirrename_test

import (
	"encoding/json"
	"testing"

	"github.com/Clementine33/preen/backend/internal/dirrename"
)

func TestMemberCategory(t *testing.T) {
	for _, tc := range []struct{ outcome, want string }{
		{`{"action":"rename"}`, "rename"},
		{`{"action":"blocked","reason_code":"X"}`, "blocked"},
		{`{"action":"noop","observed_name":"hand"}`, "noop"},
		{`{"action":"noop"}`, "unchanged"},
	} {
		if got := (&dirrename.Task{}).MemberCategory([]json.RawMessage{json.RawMessage(tc.outcome)}); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.outcome, got, tc.want)
		}
	}
	if got := (&dirrename.Task{}).MemberCategory(nil); got != "" {
		t.Errorf("no component = %q", got)
	}
}
