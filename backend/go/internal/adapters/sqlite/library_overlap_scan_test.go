package sqlite_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

func legacyOverlappingLibraries(t *testing.T) (*sqlite.Repository, string, string) {
	t.Helper()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	root := filepath.ToSlash(t.TempDir())
	inner := root + "/nested"
	// A pre-existing configuration bypasses the new creation guard.
	if _, err = repo.DB().Exec(`INSERT INTO libraries(id,name,root_path,root_path_key)
  VALUES('outer','outer',?,?),('inner','inner',?,?)`, root, root, inner, inner); err != nil {
		t.Fatal(err)
	}
	inventoryfixture.SeedInventory(t, repo,
		inventoryfixture.Row{Path: root + "/RJ123456/track.mp3", RootPath: root, Size: 5, Mtime: 10})
	return repo, root, inner
}

func TestScanRefusesLegacyOverlappingLibraries(t *testing.T) {
	for _, owner := range []string{"outer", "inner"} {
		for _, mode := range []string{"full", "folder", "selected", "service", "member", "admit full", "admit member"} {
			t.Run(owner+"/"+mode, func(t *testing.T) {
				repo, outer, inner := legacyOverlappingLibraries(t)
				root := outer
				if owner == "inner" {
					root = inner
				}
				before, err := repo.ListEntriesUnderPath(outer)
				if err != nil {
					t.Fatal(err)
				}
				walked := false
				pipeline := inventory.NewPipeline(
					sqlite.NewScanStaging(repo),
					func(context.Context, string, string, int, func(inventory.DirEntry) error) error {
						walked = true
						return nil
					},
					func(context.Context, string, string, func(inventory.DirEntry) error) error { walked = true; return nil },
					nil,
				)
				service := inventory.NewService(pipeline, repo, nil, nil)
				err = attemptOverlapScan(t.Context(), pipeline, service, mode, root)
				scanErr, ok := inventory.AsError(err)
				if !ok || scanErr.Kind != inventory.ErrKindConflict || scanErr.Code != "LIBRARY_ROOT_OVERLAP" {
					t.Fatalf("scan refusal = %v", err)
				}
				if walked {
					t.Fatal("refused scan traversed media")
				}
				after, err := repo.ListEntriesUnderPath(outer)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("inventory changed: %+v, %v", after, err)
				}
				sessions, err := repo.ListScanSessionsByRoot(root)
				if err != nil || len(sessions) != 0 {
					t.Fatalf("refused scan created sessions: %+v, %v", sessions, err)
				}
				if err = repo.CheckScanRoot(outer + "-sibling"); err != nil {
					t.Fatalf("unaffected sibling blocked: %v", err)
				}
			})
		}
	}
}

func attemptOverlapScan(
	ctx context.Context,
	pipeline *inventory.Pipeline,
	service inventory.Service,
	mode, root string,
) error {
	scope := root + "/RJ123456"
	switch mode {
	case "full":
		_, err := pipeline.ScanRootCtx(ctx, root)
		return err
	case "folder":
		_, err := pipeline.ScanFolderCtx(ctx, scope, root)
		return err
	case "selected":
		_, err := pipeline.ScanFoldersCtx(ctx, []string{scope}, root, 1)
		return err
	case "service":
		_, err := service.Scan(ctx, inventory.Request{RootPath: root}, nil)
		return err
	case "member":
		return service.RefreshMember(ctx, scope, root)
	default:
		admit := service.AdmitLibraryScan
		if mode == "admit member" {
			admit = service.AdmitMemberRefresh
		}
		release, err := admit(root)
		if release != nil {
			release()
		}
		return err
	}
}
