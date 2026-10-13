package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/collections"
	"github.com/onsei/organizer/backend/internal/works"
)

func TestCollectionsUpgradePreservesFavoritesAndPersonalData(t *testing.T) {
	for _, baseline := range []string{"initial", "previous release"} {
		t.Run(baseline, func(t *testing.T) {
			files := releaseFixture(t)
			if baseline == "previous release" {
				for _, name := range []string{"00002_main_covers.sql", "00003_work_age_category.sql", "00004_work_roots.sql", "00005_work_language.sql"} {
					data, err := os.ReadFile(filepath.Join("migrations", name))
					if err != nil {
						t.Fatal(err)
					}
					files[name] = &fstest.MapFile{Data: data}
				}
			}
			path := filepath.Join(t.TempDir(), "cache.db")
			old, err := sqlite.OpenReleaseWithMigrations(path, files)
			if err != nil {
				t.Fatal(err)
			}
			if _, seedErr := old.DB().Exec(`
			 INSERT INTO works(id,collected_at,created_at,updated_at) VALUES
			 ('RJ00000001','now','now','now'),('RJ00000002','now','now','now'),
			 ('RJ00000003','now','now','now'),('RJ00000004','now','now','now');
			 INSERT INTO work_user_data(work_id,rating,favorite,note,updated_at) VALUES
			 ('RJ00000001',8,1,'keep','2026-10-13T01:00:00.000000000Z'),
			 ('RJ00000002',NULL,1,'','2026-10-12T01:00:00.000000000Z'),
			 ('RJ00000003',NULL,1,'','2026-10-13T01:00:00.000000000Z'),
			 ('RJ00000004',9,0,'not favorite','2026-10-11T01:00:00.000000000Z');`); seedErr != nil {
				t.Fatal(seedErr)
			}
			if closeErr := old.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			upgraded, err := sqlite.NewRepository(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = upgraded.Close() })
			assertUpgradedCollections(t, upgraded)
		})
	}
}

func assertUpgradedCollections(t *testing.T, upgraded *sqlite.Repository) {
	t.Helper()
	detail, refusal := collections.NewService(upgraded).
		Detail(context.Background(), collections.FavoritesID, collections.Window{Limit: 50, Page: 1})
	if refusal != nil {
		t.Fatal(refusal)
	}
	want := []works.WorkCode{"RJ00000002", "RJ00000001", "RJ00000003"}
	if detail.Total != 3 || len(detail.Works) != 3 {
		t.Fatalf("upgraded favorites = %+v", detail)
	}
	for i, id := range want {
		if detail.Works[i].ID != id || !detail.Works[i].Favorite {
			t.Fatalf("upgraded order = %+v", detail.Works)
		}
	}
	work, found, err := upgraded.GetWork("RJ00000001")
	if err != nil || !found || work.Rating == nil || *work.Rating != 8 || work.Note != "keep" {
		t.Fatalf("personal data = %+v, found=%v, err=%v", work, found, err)
	}
	other, found, err := upgraded.GetWork("RJ00000004")
	if err != nil || !found || other.Favorite || other.Note != "not favorite" {
		t.Fatalf("nonfavorite data = %+v, found=%v, err=%v", other, found, err)
	}
}
