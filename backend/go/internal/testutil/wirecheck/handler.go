package wirecheck

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// Handler observes bytes without changing streaming, headers, status or the
// handler's request decoding. It also covers tests that bypass request helpers.
func Handler(t testing.TB, next http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request []byte
		if r.Body != nil {
			var err error
			request, err = io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(request))
		}
		observed := &responseCapture{ResponseWriter: w, status: 200}
		next.ServeHTTP(observed, r)
		var body any
		if len(request) > 0 {
			body = request
		}
		AssertHTTP(
			t,
			r.Method,
			r.URL.String(),
			observed.status,
			body,
			observed.body.Bytes(),
			w.Header().Get("Content-Type"),
		)
	})
}

type responseCapture struct {
	http.ResponseWriter

	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func (w *responseCapture) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseCapture) WriteHeader(status int) {
	if status >= 200 && !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseCapture) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	contentType := w.Header().Get("Content-Type")
	if contentType == "application/json" || contentType == "text/event-stream" {
		_, _ = w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}
