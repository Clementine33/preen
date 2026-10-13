package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectionsConcurrentAppendAndWorkDeleteKeepReferencesValid(t *testing.T) {
	server, repo := collectionServer(t)
	seedCollectionWorks(t, repo, "RJ00000001")
	path := newCollectionPath(t, server, "Concurrent")
	type result struct {
		method   string
		response *httptest.ResponseRecorder
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, action := range []struct{ method, path, body string }{
		{http.MethodPost, path + "/works", `{"rjs":["RJ00000001"]}`},
		{http.MethodDelete, "/api/v1/works/RJ00000001", ""},
	} {
		go func() {
			<-start
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(action.method, action.path, strings.NewReader(action.body)))
			results <- result{action.method, response}
		}()
	}
	close(start)
	statuses := make(map[string]int, 2)
	for range 2 {
		outcome := <-results
		statuses[outcome.method] = outcome.response.Code
	}
	switch {
	case statuses[http.MethodPost] == 200 && statuses[http.MethodDelete] == 409:
		assertCollectionWorks(t, server, path, "RJ00000001")
		collectionRequest(t, server, http.MethodGet, "/api/v1/works/RJ00000001", nil, 200)
	case statuses[http.MethodPost] == 404 && statuses[http.MethodDelete] == 204:
		assertCollectionWorks(t, server, path)
		collectionRequest(t, server, http.MethodGet, "/api/v1/works/RJ00000001", nil, 404)
	default:
		t.Fatalf("non-serial result: %v", statuses)
	}
}
