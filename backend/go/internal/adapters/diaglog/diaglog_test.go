package diaglog_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/diaglog"
)

func TestTailMatchesOutputAndHonorsLevel(t *testing.T) {
	var out bytes.Buffer
	logger, tail, err := diaglog.New(&out, "warn")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hidden")
	logger.Warn("terminal write failed", "execution_id", "ex_1")
	if !strings.Contains(out.String(), `level=WARN msg="terminal write failed" execution_id=ex_1`) {
		t.Fatalf("output = %q", out.String())
	}
	if strings.Contains(out.String(), "hidden") {
		t.Fatalf("info passed a warn level: %q", out.String())
	}
	if got := string(tail.Bytes()); got != out.String() {
		t.Fatalf("tail = %q, output = %q", got, out.String())
	}
}

func TestTailKeepsNewestWholeLinesWithinCapacity(t *testing.T) {
	logger, tail, err := diaglog.New(&bytes.Buffer{}, "")
	if err != nil {
		t.Fatal(err)
	}
	padding := strings.Repeat("x", 1000)
	for i := range 10000 {
		logger.Info("line", "i", i, "pad", padding)
	}
	got := tail.Bytes()
	if len(got) > 2<<20 || len(got) < 2<<20-2000 {
		t.Fatalf("tail holds %d bytes", len(got))
	}
	if !bytes.HasPrefix(got, []byte("time=")) || !bytes.HasSuffix(got, []byte("\n")) {
		t.Fatalf("tail is not whole lines: %q…%q", got[:20], got[len(got)-20:])
	}
	if !bytes.Contains(got, []byte("i=9999 ")) || bytes.Contains(got, []byte("i=0 ")) {
		t.Fatal("tail did not keep the newest lines")
	}
}

func TestNewRefusesUnknownLevel(t *testing.T) {
	if _, _, err := diaglog.New(&bytes.Buffer{}, "verbose"); err == nil {
		t.Fatal("verbose accepted")
	}
}
