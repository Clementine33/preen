package refresh_test

import (
	"encoding/json"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works/refresh"
)

func TestMemberCategory(t *testing.T) {
	for _, action := range []string{"refresh", "cover", "skip"} {
		raw := json.RawMessage(`{"action":"` + action + `"}`)
		if got := (&refresh.Task{}).MemberCategory([]json.RawMessage{raw}); got != action {
			t.Errorf("%s = %q", action, got)
		}
	}
	if got := (&refresh.Task{}).MemberCategory(nil); got != "" {
		t.Errorf("no component = %q", got)
	}
}
