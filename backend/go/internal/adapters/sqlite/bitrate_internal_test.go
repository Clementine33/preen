// The retry classifier and attempt cap are private to the bitrate write path and
// have no observable seam through UpdateEntryBitrates.

package sqlite

import (
	"errors"
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
