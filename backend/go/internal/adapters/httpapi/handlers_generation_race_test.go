package httpapi //nolint:testpackage // exercises the existing HTTP test server and request helpers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestConcurrentGenerationKeyHTTPContract(t *testing.T) {
	for _, changedRequest := range []bool{false, true} {
		name := "same request replays"
		if changedRequest {
			name = "changed request conflicts"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			h, repo := newWorksetServer(t, func(fn func() error) error {
				entered <- struct{}{}
				<-release
				return fn()
			})
			libID := seedLibrary(t, repo)
			seedMember(t, repo, "albumA")
			wsID := createRecord(t, h, libID, "create-generation-race")
			opPath := "/api/v1/worksets/" + wsID + "/operations/conversion"
			doc := draftDocumentFixture()
			settingsPath := "/api/v1/tasks/conversion/settings"
			settings := req(t, h, http.MethodGet, settingsPath, testToken, nil)
			if settings.Code != http.StatusOK {
				t.Fatalf("settings read: %d %s", settings.Code, settings.Body.String())
			}
			savedSettings := reqWithIfMatch(t, h, http.MethodPut, settingsPath, testToken,
				map[string]any{"settings": commonOf(doc)}, settings.Header().Get("ETag"))
			if savedSettings.Code != http.StatusOK {
				t.Fatalf("settings save: %d %s", savedSettings.Code, savedSettings.Body.String())
			}
			version := generationRaceVersion(t, h, opPath)
			saved := reqWithIfMatch(t, h, http.MethodPut, opPath+"/draft", testToken, doc, strconv.Itoa(version))
			if saved.Code != http.StatusOK {
				t.Fatalf("draft save: %d %s", saved.Code, saved.Body.String())
			}
			version = generationRaceVersion(t, h, opPath)

			responses := make(chan *httptest.ResponseRecorder, 2)
			start := func(version int) {
				go func() {
					responses <- reqWithIdempotencyAndIfMatch(t, h, http.MethodPost, opPath+"/revisions",
						testToken, nil, "shared-key", strconv.Itoa(version))
				}()
			}
			awaitEntry := func() {
				t.Helper()
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("generation request did not reach the enqueue guard")
				}
			}
			start(version)
			awaitEntry()
			if changedRequest {
				members, err := repo.ListWorksetMembers(wsID)
				if err != nil || len(members) != 1 {
					t.Fatalf("members: %+v err=%v", members, err)
				}
				doc["members"] = []any{map[string]any{
					"member_id": members[0].MemberID,
					"overrides": map[string]any{"no_effect_tags": []string{"changed"}},
				}}
				saved = reqWithIfMatch(t, h, http.MethodPut, opPath+"/draft", testToken, doc,
					strconv.Itoa(version))
				if saved.Code != http.StatusOK {
					t.Fatalf("changed draft save: %d %s", saved.Code, saved.Body.String())
				}
				version = generationRaceVersion(t, h, opPath)
			}
			start(version)
			awaitEntry()
			close(release)
			assertGenerationKeyHTTPRace(t, responses, changedRequest)
		})
	}
}

func assertGenerationKeyHTTPRace(
	t *testing.T, responses <-chan *httptest.ResponseRecorder, changedRequest bool,
) {
	t.Helper()
	var created, replayed, conflicts int
	var generationIDs []string
	for range 2 {
		response := <-responses
		var body struct {
			Created    bool `json:"created"`
			Generation struct {
				GenerationID string `json:"generation_id"`
			} `json:"generation"`
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		switch {
		case response.Code == http.StatusAccepted && body.Created:
			created++
			generationIDs = append(generationIDs, body.Generation.GenerationID)
		case response.Code == http.StatusAccepted && !body.Created:
			replayed++
			generationIDs = append(generationIDs, body.Generation.GenerationID)
		case response.Code == http.StatusConflict && body.Code == "IDEMPOTENCY_KEY_REUSED":
			conflicts++
		default:
			t.Fatalf("unexpected start response: %d %s", response.Code, response.Body.String())
		}
	}
	if created != 1 || (changedRequest && conflicts != 1) || (!changedRequest && replayed != 1) {
		t.Fatalf("created=%d replayed=%d conflicts=%d", created, replayed, conflicts)
	}
	if len(generationIDs) == 0 || generationIDs[0] == "" ||
		(!changedRequest && generationIDs[0] != generationIDs[1]) {
		t.Fatalf("generation IDs = %v", generationIDs)
	}
}

func generationRaceVersion(t *testing.T, h http.Handler, opPath string) int {
	t.Helper()
	response := req(t, h, http.MethodGet, opPath, testToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("operation read: %d %s", response.Code, response.Body.String())
	}
	var operation struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &operation); err != nil {
		t.Fatal(err)
	}
	return operation.Version
}
