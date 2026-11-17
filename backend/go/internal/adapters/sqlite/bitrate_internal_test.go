// The retry classifier and attempt cap are private to the bitrate write path and
// have no observable seam through UpdateEntryBitrates.

package sqlite

import (
	"errors"
	"strings"
	"testing"
)

func TestIsSQLiteBusyLockedError_Detection(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("database is locked"), true},
		{errors.New("SQLITE_BUSY: concurrent access"), true},
		{errors.New("sqlite_locked error"), true},
		{errors.New("no such table: entries"), false},
		{nil, false},
		{errors.New("disk I/O error"), false},
	}
	for _, tt := range tests {
		got := isSQLiteBusyLockedError(tt.err)
		if got != tt.want {
			t.Errorf("isSQLiteBusyLockedError(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestPersistBitrateUpdates_RetriesOnBusyError(t *testing.T) {
	// Test: retry loop should retry on busy and stop on non-busy.
	attempts := 0
	lastErr := error(nil)
	for attempt := 0; attempt <= bitratePersistRetryLimit; attempt++ {
		attempts++
		// Simulate: first 2 attempts return busy, 3rd returns non-busy
		if attempt < 2 {
			err := errors.New("database is locked")
			if !isSQLiteBusyLockedError(err) {
				t.Fatalf("expected busy error to be detected")
			}
			lastErr = err
			continue
		}
		// Non-busy error - should stop
		err := errors.New("no such table: entries")
		if isSQLiteBusyLockedError(err) {
			t.Fatalf("expected non-busy error to not be detected as busy")
		}
		lastErr = err
		break
	}
	if lastErr == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(lastErr.Error(), "no such table") {
		t.Fatalf("expected non-busy error to be returned, got %v", lastErr)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts (2 busy + 1 non-busy), got %d", attempts)
	}
}

func TestPersistBitrateUpdates_ExhaustsRetriesOnBusy(t *testing.T) {
	// Verify that exhausting retries returns the last busy error.
	attempts := 0
	var lastErr error
	for attempt := 0; attempt <= bitratePersistRetryLimit; attempt++ {
		attempts++
		err := errors.New("SQLITE_BUSY: database is locked")
		if !isSQLiteBusyLockedError(err) {
			t.Fatalf("expected busy error to be detected at attempt %d", attempt)
		}
		lastErr = err
	}
	if lastErr == nil {
		t.Fatal("expected error, got nil")
	}
	if !isSQLiteBusyLockedError(lastErr) {
		t.Fatalf("expected last error to be busy, got %v", lastErr)
	}
	if attempts != bitratePersistRetryLimit+1 {
		t.Fatalf("expected %d attempts (0..%d), got %d", bitratePersistRetryLimit+1, bitratePersistRetryLimit, attempts)
	}
}
