package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
)

func TestDiagnosticLogExportIsAuthenticatedAndHeaded(t *testing.T) {
	handler := httpapi.NewServer(httpapi.Dependencies{
		Token: "secret", Version: "1.2.3",
		StartedAt:     time.Date(2026, 10, 26, 1, 2, 3, 0, time.UTC),
		DiagnosticLog: func() []byte { return []byte("time=t level=INFO msg=\"preen listening\"\n") },
	})
	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/log", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", anonymous.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/log", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("status = %d, type = %q", w.Code, w.Header().Get("Content-Type"))
	}
	disposition := w.Header().Get("Content-Disposition")
	if !regexp.MustCompile(`^attachment; filename="preen-1\.2\.3-\d{8}T\d{6}Z\.log"$`).MatchString(disposition) {
		t.Fatalf("disposition = %q", disposition)
	}
	lines := strings.Split(w.Body.String(), "\n")
	if !strings.HasPrefix(lines[0], "# preen 1.2.3 go") || lines[1] != "# started 2026-10-26T01:02:03Z" ||
		!strings.HasPrefix(lines[2], "# exported ") || lines[3] != `time=t level=INFO msg="preen listening"` {
		t.Fatalf("body = %q", w.Body.String())
	}
}
