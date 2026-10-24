package httpapi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/cleanup"
)

func TestCleanupHTTPValidationAdmissionAndAuth(t *testing.T) {
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	gate := admission.NewGate(nil)
	service := cleanup.NewService(
		context.Background(),
		repo,
		gate,
		cleanup.Options{},
	)
	defer service.Shutdown()
	handler := httpapi.NewServer(httpapi.Dependencies{Cleanup: service, Token: "secret"})
	request := func(method, path, body, credential string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	for _, body := range []string{`{"categories":[]}`, `{"categories":["unknown"]}`, `{"categories":["scan_sessions"],"request_id":"no"}`} {
		if rec := request("POST", "/api/v1/maintenance/cleanup/preview", body, "secret"); rec.Code != 400 {
			t.Fatalf("invalid body %s => %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := request("GET", "/api/v1/maintenance/cleanup", "", ""); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"http://local/api/v1/maintenance/cleanup/preview",
		bytes.NewBufferString(`{"categories":["scan_sessions"]}`),
	)
	req.AddCookie(
		&http.Cookie{
			Name:     "preen_token",
			Value:    base64.RawURLEncoding.EncodeToString([]byte("secret")),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		},
	)
	req.Header.Set("Origin", "http://other")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal(rec.Code)
	}
	rec = request("GET", "/api/v1/maintenance/cleanup", "", "secret")
	if rec.Code != 200 || rec.Body.String() != `{"run":null}` {
		t.Fatalf("empty %d %s", rec.Code, rec.Body)
	}
	rec = request(
		"POST",
		"/api/v1/maintenance/cleanup/preview",
		`{"categories":["scan_sessions","scan_sessions"]}`,
		"secret",
	)
	var preview cleanup.Preview
	if decodeErr := json.Unmarshal(rec.Body.Bytes(), &preview); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if rec.Code != 200 || len(preview.Categories) != 1 {
		t.Fatalf("preview %d %s", rec.Code, rec.Body)
	}
	release, err := gate.BeginMaintenance()
	if err != nil {
		t.Fatal(err)
	}
	rec = request("POST", "/api/v1/maintenance/cleanup", `{"categories":["scan_sessions"]}`, "secret")
	if rec.Code != 409 {
		t.Fatalf("busy %d %s", rec.Code, rec.Body)
	}
	release()
	rec = request("POST", "/api/v1/maintenance/cleanup", `{"categories":["scan_sessions"]}`, "secret")
	if rec.Code != 202 {
		t.Fatalf("start %d %s", rec.Code, rec.Body)
	}
	var accepted struct {
		Run cleanup.Run `json:"run"`
	}
	if decodeErr := json.Unmarshal(rec.Body.Bytes(), &accepted); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	for range 2 {
		stopped := request("POST", "/api/v1/maintenance/cleanup/"+accepted.Run.ID+"/cancel", "", "secret")
		if stopped.Code != 200 {
			t.Fatalf("cancel %d %s", stopped.Code, stopped.Body)
		}
	}
	rec = request("POST", "/api/v1/maintenance/cleanup/missing/cancel", "", "secret")
	if rec.Code != 404 {
		t.Fatalf("missing %d %s", rec.Code, rec.Body)
	}
}
