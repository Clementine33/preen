package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/works"
)

func TestInitialReleaseCollectionsPreservePersonalData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	repo, err := sqlite.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if _, err := repo.DB().Exec(`
        INSERT INTO works(id,collected_at,created_at,updated_at) VALUES
        ('RJ00000001','now','now','now'),('RJ00000002','now','now','now'),
        ('RJ00000003','now','now','now'),('RJ00000004','now','now','now');
        INSERT INTO work_user_data(work_id,rating,note) VALUES
        ('RJ00000001',8,'keep'),('RJ00000004',9,'not favorite');
        INSERT INTO collection_works(collection_id,work_id,position) VALUES
        ('favorites','RJ00000002',0),('favorites','RJ00000001',1),('favorites','RJ00000003',2);`); err != nil {
		t.Fatal(err)
	}
	assertUpgradedCollections(t, repo)
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
