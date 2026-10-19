package app

import "fmt"

// BuildHandshakeLine is the ONE line the backend prints on stdout when it is
// ready. The dev and e2e launchers parse it for the HTTP port.
func BuildHandshakeLine(version string, httpPort int) string {
	return fmt.Sprintf("PREEN_BACKEND_READY version=%s http_port=%d", version, httpPort)
}
