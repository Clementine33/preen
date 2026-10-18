package library_test

import (
	"testing"

	"github.com/Clementine33/preen/backend/internal/library"
)

func TestStorageClassReservesCPUForOtherWork(t *testing.T) {
	for _, tc := range []struct {
		class               library.StorageClass
		cpus, media, encode int
	}{
		{library.StorageHDD, 1, 1, 1}, {library.StorageSSD, 2, 1, 1},
		{library.StorageHDD, 6, 4, 4}, {library.StorageSSD, 6, 4, 4},
		{library.StorageHDD, 12, 8, 4}, {library.StorageSSD, 12, 8, 8},
		{library.StorageHDD, 48, 8, 4}, {library.StorageSSD, 48, 16, 32},
		{"", 12, 8, 4},
	} {
		if got := tc.class.MediaWorkers(tc.cpus); got != tc.media {
			t.Errorf("%q/%d CPUs media=%d, want %d", tc.class, tc.cpus, got, tc.media)
		}
		if got := tc.class.EncodeWorkers(tc.cpus); got != tc.encode {
			t.Errorf("%q/%d CPUs encode=%d, want %d", tc.class, tc.cpus, got, tc.encode)
		}
	}
}
