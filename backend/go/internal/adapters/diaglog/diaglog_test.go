package diaglog_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/diaglog"
)

// export splits Log Export into its header line and the kept lines.
func export(t *diaglog.Tail) (string, []byte) {
	header, lines, _ := bytes.Cut(t.Export(), []byte("\n"))
	return string(header), lines
}

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
	header, lines := export(tail)
	if header != "# log level=warn dropped=0 skipped=0" {
		t.Fatalf("header = %q", header)
	}
	if string(lines) != out.String() {
		t.Fatalf("tail = %q, output = %q", lines, out.String())
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
	header, got := export(tail)
	if !strings.HasPrefix(header, "# log level=info dropped=") || strings.HasSuffix(header, "dropped=0 skipped=0") {
		t.Fatalf("header = %q", header)
	}
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

// Dropping makes exactly the room the new line needs: a cut that already falls
// on a line boundary costs no extra line.
func TestTailDropsOnlyTheOldestLinesItMust(t *testing.T) {
	line := func(c byte) []byte { return append(bytes.Repeat([]byte{c}, 1<<20-1), '\n') }
	tail := &diaglog.Tail{}
	for _, c := range []byte("abc") {
		_, _ = tail.Write(line(c))
	}
	header, got := export(tail)
	if !bytes.Equal(got, append(line('b'), line('c')...)) {
		t.Fatalf("kept %d bytes starting %q, want the last two lines", len(got), got[:1])
	}
	if !strings.HasSuffix(header, " dropped=1 skipped=0") {
		t.Fatalf("header = %q", header)
	}
}

func TestTailSkipsALineLargerThanItself(t *testing.T) {
	tail := &diaglog.Tail{}
	_, _ = tail.Write([]byte("kept\n"))
	if n, err := tail.Write(append(bytes.Repeat([]byte{'x'}, 20<<20), '\n')); n != 20<<20+1 || err != nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	if header, got := export(tail); string(got) != "kept\n" || !strings.HasSuffix(header, " dropped=0 skipped=1") {
		t.Fatalf("header = %q, tail = %.20q", header, got)
	}
}

func TestNewAcceptsOnlyTheFourLevels(t *testing.T) {
	for _, level := range []string{"verbose", "ERROR+1", "info-4", "0"} {
		if _, _, err := diaglog.New(&bytes.Buffer{}, level); err == nil {
			t.Fatalf("%q accepted", level)
		}
	}
	var out bytes.Buffer
	logger, _, err := diaglog.New(&out, "ERROR")
	if err != nil {
		t.Fatal(err)
	}
	logger.Warn("hidden")
	logger.Error("shown")
	if got := out.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "shown") {
		t.Fatalf("output = %q", got)
	}
}
