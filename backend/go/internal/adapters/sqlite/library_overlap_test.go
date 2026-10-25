package sqlite_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/library"
)

func TestLibraryCreateRejectsNestedRoots(t *testing.T) {
	for _, tc := range []struct {
		first, second string
		overlap       bool
	}{
		{"/a", "/a/nested", true}, {"/a/nested", "/a", true},
		{"/", "/a", true}, {"C:/", "c:/Music", true},
		{"C:/Music", "c:/MUSIC/album", true},
		{"//server/share", "//SERVER/SHARE/album", true},
		{"/a", "/a b", false}, {"/a", "/ab", false}, {"/Rock", "/rock", false},
		{"C:/Music", "c:/Music b", false}, {"C:/Music", "D:/Music", false},
		{"//server/share", "//server/share2", false},
	} {
		t.Run(tc.first+" then "+tc.second, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			service := library.NewService(repo, nil, repo, nil)
			if _, err = service.Create("first", tc.first); err != nil {
				t.Fatal(err)
			}
			_, err = service.Create("second", tc.second)
			if tc.overlap && !errors.Is(err, library.ErrLibraryRootOverlap) || !tc.overlap && err != nil {
				t.Fatalf("create = %v, overlap=%v", err, tc.overlap)
			}
			libraries, err := service.List()
			want := 2
			if tc.overlap {
				want = 1
			}
			if err != nil || len(libraries) != want {
				t.Fatalf("libraries = %+v, %v", libraries, err)
			}
		})
	}
}

func TestLibraryRootChangeRejectsOverlap(t *testing.T) {
	for _, tc := range []struct {
		existing, target string
		want             error
	}{
		{"/a", "/a/nested", library.ErrLibraryRootOverlap},
		{"/a/nested", "/a", library.ErrLibraryRootOverlap},
		{"/a", "/a/.", library.ErrLibraryExists},
	} {
		t.Run(tc.target, func(t *testing.T) {
			repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			service := library.NewService(repo, nil, repo, nil)
			if _, err = service.Create("existing", tc.existing); err != nil {
				t.Fatal(err)
			}
			sibling, err := service.Create("sibling", "/b")
			if err != nil {
				t.Fatal(err)
			}
			changedName := "must roll back"
			if _, err = service.Patch(sibling.ID, &changedName, &tc.target, nil); !errors.Is(err, tc.want) {
				t.Fatalf("root change = %v, want %v", err, tc.want)
			}
			unchanged, err := service.Get(sibling.ID)
			if err != nil || !reflect.DeepEqual(sibling, unchanged) {
				t.Fatalf("library changed after refusal: %+v, %v", unchanged, err)
			}
		})
	}
}

func TestConcurrentLibraryCreatesCannotIntroduceOverlap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	first, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := sqlite.NewDevelopmentRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, repo := range []*sqlite.Repository{first, second} {
		root := []string{"/a", "/a/nested"}[i]
		go func() { <-start; _, createErr := repo.CreateLibrary("library", root); results <- createErr }()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, library.ErrLibraryRootOverlap):
			conflicts++
		default:
			t.Fatalf("create = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestLegacyOverlappingLibrariesCanBeRepaired(t *testing.T) {
	for _, action := range []string{"delete", "move"} {
		t.Run(action, func(t *testing.T) {
			repo, root, inner := legacyOverlappingLibraries(t)
			service := library.NewService(repo, nil, repo, nil)
			if _, err := service.Create("duplicate", root+"/."); !errors.Is(err, library.ErrLibraryExists) {
				t.Fatalf("canonical duplicate changed error in legacy config: %v", err)
			}
			name, class := "renamed", library.StorageSSD
			unchanged := inner + "/."
			if _, err := service.Patch("inner", &name, &unchanged, &class); err != nil {
				t.Fatal(err)
			}
			if action == "delete" {
				if err := service.Delete("inner"); err != nil {
					t.Fatal(err)
				}
			} else {
				moved := root + "-sibling"
				if _, err := service.Patch("inner", nil, &moved, nil); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.CheckScanRoot(root); err != nil {
				t.Fatalf("root remains blocked after repair: %v", err)
			}
		})
	}
}
