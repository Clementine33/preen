package execute_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/conversion/reconcile"
)

func TestFrozenFactsToleranceAndUnspecifiedFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "source.wav")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Windows FILETIME stores timestamps in 100ns units.
	const step = 100 * time.Nanosecond
	for _, delta := range []time.Duration{
		-time.Second - step, -time.Second, -time.Second + step,
		0, time.Second - step, time.Second, time.Second + step,
	} {
		mtime := time.Unix(1700000000, 0).Add(delta)
		if err := os.Chtimes(file, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		pathInfo, err := os.Lstat(file)
		if err != nil {
			t.Fatal(err)
		}
		handle, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		handleInfo, err := handle.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := handle.Close(); err != nil {
			t.Fatal(err)
		}
		want := delta >= -time.Second && delta <= time.Second
		for _, info := range []os.FileInfo{pathInfo, handleInfo} {
			if got := execute.CompareFrozenFacts(
				reconcile.FileTuple{Size: 5, Mtime: 1700000000},
				info,
			) == nil; got != want {
				t.Fatalf("delta %s matches = %t, want %t", delta, got, want)
			}
			if err := execute.CompareFrozenFacts(reconcile.FileTuple{}, info); err != nil {
				t.Fatalf("unspecified facts: %v", err)
			}
			if err := execute.CompareFrozenFacts(reconcile.FileTuple{Size: 10}, info); err == nil {
				t.Fatal("wrong size accepted")
			}
		}
	}
}
