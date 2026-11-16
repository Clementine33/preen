//nolint:testpackage // the package-wide audit includes private handler fixtures
package httpapi

import (
	"fmt"
	"os"
	"testing"

	"github.com/Clementine33/preen/backend/internal/testutil/wirecheck"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := wirecheck.WriteCoverage(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}
