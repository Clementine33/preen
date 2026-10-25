package fileops_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"

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

func TestRenameExtensionRefreshesOverviewAudioCounts(t *testing.T) {
	root := t.TempDir()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "rename.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	gate := admission.NewGate(repo.HasActiveSession)
	scan := inventory.NewService(
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
		scan.RefreshMember,
		filesystem.Recover,
		filesystem.RenameNoReplace,
		filesystem.IsResolvedWithinRoot,
	)
	f.mkdir("albumA")
	f.write("albumA/track.MP3", "audio")
	member := filepath.ToSlash(filepath.Join(root, "albumA"))
	inventoryfixture.SeedInventory(
		t,
		repo,
		inventoryfixture.Row{
			Path:       member,
			RootPath:   filepath.ToSlash(root),
			IsDir:      true,
			IsWorkRoot: true,
			CreatedAt:  1767323045,
		},
	)
	if err = scan.RefreshMember(t.Context(), filepath.Join(root, "albumA"), root); err != nil {
		t.Fatal(err)
	}
	assertCounts := func(want int) {
		t.Helper()
		dirs, err := repo.ListLibraryDirs(root)
		if err != nil || len(dirs) != 1 {
			t.Fatalf("dirs=%+v err=%v", dirs, err)
		}
		d := dirs[0]
		if d.AudioFileCount != want || d.FileCount != 1 ||
			!d.AddedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
			t.Fatalf("overview=%+v", d)
		}
		counts, err := repo.DirAudioCounts(root, []string{"albumA"})
		if err != nil || counts["albumA"] != want {
			t.Fatalf("counts=%v err=%v", counts, err)
		}
	}
	assertCounts(1)
	for _, names := range [][2]string{{"track.MP3", "track.txt"}, {"track.txt", "track.FLAC"}} {
		result := f.apply(
			fileops.Request{
				MemberPath: "albumA",
				Operation:  fileops.OpRename,
				Items:      []fileops.Item{{Source: names[0], Name: names[1]}},
			},
		)
		if result.Succeeded != 1 || !result.Refresh.OK {
			t.Fatalf("rename=%+v", result)
		}
		want := 0
		if names[1] == "track.FLAC" {
			want = 1
		}
		assertCounts(want)
	}
}
