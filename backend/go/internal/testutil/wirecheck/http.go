package wirecheck

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// AssertHTTP checks successful request bodies and every JSON response. Invalid
// request fixtures used to test refusals are deliberately not accepted as inputs.
func AssertHTTP(t testing.TB, method, address string, status int, request any, response []byte, contentType string) {
	t.Helper()
	if status >= 400 {
		if strings.Contains(contentType, "application/json") {
			name := "errorResponse"
			if strings.Contains(string(response), `"details"`) {
				name = "errorWithDetails"
			}
			AssertSchema(t, name, response)
		}
		return
	}
	v := cached()
	if v.err != nil {
		t.Error(v.err)
		return
	}
	u, err := url.Parse(address)
	if err != nil {
		t.Error(err)
		return
	}
	binding, kind, found := findHTTPContract(v.contracts, method, u.Path)
	if !found {
		if strings.Contains(contentType, "application/json") {
			t.Errorf("missing wire contract: %s %s", method, u.Path)
		}
		return
	}
	if request != nil && binding.Request != "" {
		root := binding.Request
		if strings.HasSuffix(u.Path, "/draft") && kind != "conversion" {
			for _, task := range []string{"rename", "metadata_refresh"} {
				if task == kind {
					prefix := "dirrename"
					if task == "metadata_refresh" {
						prefix = "refresh"
					}
					root = prefix + "_Draft_request"
				}
			}
		}
		AssertSchema(t, selectTaskSchema(root, kind), request)
	}
	observed.Store(binding.pattern+" "+strconv.Itoa(status), true)
	name, exists := binding.Responses[strconv.Itoa(status)]
	if !exists {
		t.Errorf("unmapped wire status: %s %s: %d", method, u.Path, status)
		return
	}
	if name == "@empty" || status == 204 {
		return
	}
	if name == "@sse" || name == "@scan" {
		assertSSE(t, u.RequestURI(), kind, response)
		return
	}
	if name == "executionViewResponse" {
		name = "executionFullViewResponse"
		if u.Query().Get("view") == "members" {
			name = "executionMembersViewResponse"
		} else if u.Query().Get("member_id") != "" {
			name = "executionMemberViewResponse"
		}
	}
	AssertSchema(t, selectTaskSchema(name, kind), response)
}

func selectTaskSchema(name, kind string) string {
	if kind != "" {
		candidate := kind + "_v1_" + name
		if _, exists := cached().schemas[candidate]; exists {
			return candidate
		}
	}
	return name
}

func findHTTPContract(m manifest, method, path string) (contract, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var best contract
	bestKind, bestWild, found := "", 0, false
	for pattern, c := range m.HTTP {
		verb, route, _ := strings.Cut(pattern, " ")
		if verb != method {
			continue
		}
		segments := strings.Split(strings.Trim(route, "/"), "/")
		if len(segments) != len(parts) {
			continue
		}
		match := true
		kind := ""
		wild := 0
		for i, seg := range segments {
			if strings.HasPrefix(seg, "{") {
				wild++
				if seg == "{type}" {
					kind = parts[i]
				}
				continue
			}
			if seg != parts[i] {
				match = false
				break
			}
		}
		// The most literal route wins, as the server's mux does:
		// /works/random over /works/{rj}.
		if match && (!found || wild < bestWild) {
			c.pattern = pattern
			best, bestKind, bestWild, found = c, kind, wild, true
		}
	}
	return best, bestKind, found
}

func assertSSE(t testing.TB, path, kind string, body []byte) {
	t.Helper()
	family := "scan"
	switch {
	case strings.Contains(path, "planning-sessions"):
		family = "generation"
	case strings.Contains(path, "/executions/"):
		family = "execution"
	case strings.Contains(path, "media-facts"):
		family = "media"
	}
	event := "message"
	var data []string
	dispatch := func() {
		if len(data) == 0 {
			return
		}
		name, ok := cached().contracts.SSE[family][event]
		if !ok {
			t.Error(fmt.Errorf("unmapped %s SSE event %q", family, event))
		} else {
			if name == "executionViewResponse" {
				name = "executionFullViewResponse"
				if strings.Contains(path, "view=members") {
					name = "executionMembersViewResponse"
				}
			}
			AssertSchema(t, selectTaskSchema(name, kind), []byte(strings.Join(data, "\n")))
			observed.Store("SSE "+family+" "+event, true)
		}
		data = nil
		event = "message"
	}
	for line := range strings.SplitSeq(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		switch {
		case line == "":
			dispatch()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	dispatch()
}
