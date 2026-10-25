package fileops_test

import (
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/fileops"
	"github.com/Clementine33/preen/backend/internal/inventory"
)

// TestTheInternalRefreshIsNotRefusedByTheSlotItHolds pins the rule that makes
// the post-write refresh work at all: a file operation holds the
// direct-file-management slot, and the refresh it runs inside that slot must
// not ask for a scanning slot of its own — asking would be refused (BUSY) and a
// perfectly good write would come back as a failed refresh. The refresh runs
// through the real scanning entry here, so a future change that moves admission
// into the refresh path fails this test instead of production.
func TestTheInternalRefreshIsNotRefusedByTheSlotItHolds(t *testing.T) {
	root := t.TempDir()
	member := filepath.Join(root, "albumA")

	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "fileops-refresh.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	gate := admission.NewGate(repo.HasActiveSession)
	scanSvc := inventory.NewService(
		inventory.NewPipeline(
			sqlite.NewScanStaging(repo),
			filesystem.WalkRootEntriesParallel,
			filesystem.WalkFolderEntries,
			nil,
		),
		repo,
		gate,
		repo.UpdateLibraryScanState,
	)

	f := &fixture{t: t, root: root, gate: gate}
	f.svc = fileops.NewService(
		gate,
		scanSvc.RefreshMember,
		filesystem.Recover,
		filesystem.RenameNoReplace,
		filesystem.IsResolvedWithinRoot,
	)
	f.mkdir("albumA")
	f.write("albumA/01.flac", "audio")

	result := f.apply(fileops.Request{
		MemberPath: "albumA",
		Operation:  fileops.OpRename,
		Items:      []fileops.Item{{Source: "01.flac", Name: "02.flac"}},
	})
	if result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("counts = %+v, want the rename to succeed", result)
	}
	if !result.Refresh.OK {
		t.Fatalf(
			"refresh = %+v: the refresh a file operation runs holds the slot already; "+
				"asking for another one refuses the operation's own refresh",
			result.Refresh,
		)
	}
	if !f.exists("albumA/02.flac") {
		t.Fatal("the rename must have landed")
	}

	// The refresh really scanned the member: its rows are in the inventory.
	if found, err := repo.RootExistsInInventory(
		filepath.ToSlash(filepath.Join(member, "02.flac")),
	); err != nil ||
		!found {
		t.Fatalf("renamed file missing from inventory: found=%v err=%v", found, err)
	}
}
