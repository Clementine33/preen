package httpapi //nolint:testpackage // white-box tests exercise unexported internals

import (
	"bytes"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func decodeJSONError(r *http.Request, dst any) error {
	w := httptest.NewRecorder()
	return decodeJSON(w, r, dst)
}

func TestDecodeJSON(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if err := decodeJSONError(req, &struct{}{}); err == nil {
			t.Fatal("empty body should fail")
		}
	})

	t.Run("oversized body rejected with 413", func(t *testing.T) {
		body := `{"name":"` + strings.Repeat("x", maxRequestBodyBytes) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		err := decodeJSONError(req, &struct {
			Name string `json:"name"`
		}{})
		var pe *payloadError
		if !errors.As(err, &pe) {
			t.Fatalf("expected payloadError, got %T: %v", err, err)
		}
		if pe.status != http.StatusRequestEntityTooLarge || pe.code != "PAYLOAD_TOO_LARGE" {
			t.Fatalf("expected 413 PAYLOAD_TOO_LARGE, got %d %s", pe.status, pe.code)
		}
	})
}

func TestRecoveryMiddlewareReturnsInternalEnvelope(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if got := recorder.Body.String(); got != `{"code":"INTERNAL","message":"internal server error"}` {
		t.Fatalf("body = %q", got)
	}
}

// The access log keeps the query out (it can carry access_token) and raises
// only a server failure above debug.
func TestRecoveryMiddlewareLogsRequestsWithoutQuery(t *testing.T) {
	var logs bytes.Buffer
	// SetDefault also redirects the log package, and restoring slog's own
	// default does not undo that, so the log package is restored too.
	previous, output, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous); log.SetOutput(output); log.SetFlags(flags) })
	for path, status := range map[string]int{"/ok": http.StatusOK, "/gone": http.StatusNotFound, "/fail": http.StatusBadGateway} {
		handler := recoveryMiddleware(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }),
		)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path+"?access_token=secret", nil))
	}
	got := logs.String()
	if strings.Contains(got, "secret") {
		t.Fatalf("query logged: %s", got)
	}
	for _, want := range []string{
		`level=DEBUG msg="http request" method=GET path=/ok status=200`,
		`level=DEBUG msg="http request" method=GET path=/gone status=404`,
		`level=ERROR msg="http request" method=GET path=/fail status=502`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
}

// A 500's cause reaches the access log line of its request, never the response.
func TestRecoveryMiddlewareLogsTheCauseOfA500(t *testing.T) {
	var logs bytes.Buffer
	previous, output, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous); log.SetOutput(output); log.SetFlags(flags) })
	cause := errors.New("database is locked")
	for _, write := range []func(http.ResponseWriter){
		func(w http.ResponseWriter) { writeInternal(w, cause, "failed to list libraries") },
		func(w http.ResponseWriter) {
			writeWorksError(w, &works.Error{Kind: works.KindInternal, Code: "INTERNAL", Message: "catalog failed", Cause: cause})
		},
		func(w http.ResponseWriter) {
			writeWorksetError(w, workset.NewError(workset.ErrKindInternal, "INTERNAL", "workset failed", cause))
		},
	} {
		logs.Reset()
		recorder := httptest.NewRecorder()
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { write(w) })
		recoveryMiddleware(
			corsMiddleware(nil)(handler),
		).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/fail", nil))
		if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "locked") {
			t.Fatalf("response = %d %s", recorder.Code, recorder.Body)
		}
		got := logs.String()
		want := `level=ERROR msg="http request" method=GET path=/fail status=500`
		if !strings.Contains(got, want) || !strings.Contains(got, "database is locked") ||
			strings.Count(got, "\n") != 1 {
			t.Fatalf("log = %s", got)
		}
	}
}

type informationalResponseWriter struct {
	header   http.Header
	statuses []int
	body     bytes.Buffer
}

func (w *informationalResponseWriter) Header() http.Header { return w.header }
func (w *informationalResponseWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
}
func (w *informationalResponseWriter) Write(payload []byte) (int, error) {
	return w.body.Write(payload)
}

func TestRecoveryMiddlewareCanRecoverAfterEarlyHints(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		panic("boom")
	}))
	writer := &informationalResponseWriter{header: make(http.Header)}
	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/", nil))

	if got, want := writer.statuses, []int{
		http.StatusEarlyHints,
		http.StatusInternalServerError,
	}; !reflect.DeepEqual(
		got,
		want,
	) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if got := writer.body.String(); got != `{"code":"INTERNAL","message":"internal server error"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestRecoveryMiddlewareDoesNotReplaceSwitchingProtocols(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
		panic("boom")
	}))
	writer := &informationalResponseWriter{header: make(http.Header)}
	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/", nil))

	if got, want := writer.statuses, []int{http.StatusSwitchingProtocols}; !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if writer.body.Len() != 0 {
		t.Fatalf("body = %q, want empty", writer.body.String())
	}
}
