package httpapi

import (
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// getDiagnosticLog handles GET /api/v1/diagnostics/log: Log Export, the recent
// Diagnostic Log as a downloadable file headed by the facts a bug report
// always asks for (docs/api/diagnostics.md#log-export).
func (s *Server) getDiagnosticLog(w http.ResponseWriter, _ *http.Request) {
	exported := time.Now().UTC()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="preen-%s-%s.log"`, s.deps.Version, exported.Format("20060102T150405Z"),
	))
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "# preen %s %s %s/%s\n# started %s\n# exported %s\n",
		s.deps.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH,
		s.deps.StartedAt.UTC().Format(time.RFC3339), exported.Format(time.RFC3339))
	if s.deps.DiagnosticLog != nil {
		_, _ = w.Write(s.deps.DiagnosticLog())
	}
}
