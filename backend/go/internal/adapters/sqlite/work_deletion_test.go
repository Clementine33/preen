package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/works"
)

const deletionWork works.WorkCode = "RJ00000001"
const deletionStamp = "2026-09-27T10:00:00.000000000Z"

func deletionRepository(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo, err := sqlite.NewRepository(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	deletionSQL(
		t,
		repo,
		`INSERT INTO works(id,collected_at,updated_at) VALUES(?,?,?)`,
		string(deletionWork),
		deletionStamp,
		deletionStamp,
	)
	return repo
}

func deletionSQL(t *testing.T, repo *sqlite.Repository, query string, args ...any) {
	t.Helper()
	if _, err := repo.DB().Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestWorkDeletionProtectsPersonalData(t *testing.T) {
	for _, seam := range []string{"repository", "service"} {
		for _, tc := range []struct{ name, query, detail string }{
			{"rating", `INSERT INTO work_user_data(work_id,rating) VALUES('RJ00000001',8)`, "a personal rating is saved for this work"},
			{"note", `INSERT INTO work_user_data(work_id,note) VALUES('RJ00000001','remember')`, "a personal note is saved for this work"},
			{"whitespace note", `INSERT INTO work_user_data(work_id,note) VALUES('RJ00000001',' ')`, "a personal note is saved for this work"},
			{"user tag", `INSERT INTO user_tags VALUES('tag','keep'); INSERT INTO work_user_tags VALUES('RJ00000001','tag')`, "user tags are attached to this work"},
			{"favorite", `INSERT INTO collection_works VALUES('favorites','RJ00000001',0)`, "a collection holds this work: favorites Favorites"},
			{"collection", `INSERT INTO collections VALUES('custom','Sleep','',''); INSERT INTO collection_works VALUES('custom','RJ00000001',0)`, "a collection holds this work: custom Sleep"},
			{"position at zero", `INSERT INTO work_listening VALUES('RJ00000001','','track',0,0,1,NULL,'client',1,'')`, "listening records are saved for this work"},
			{"finished", `INSERT INTO work_listening VALUES('RJ00000001','',NULL,NULL,NULL,NULL,'finished','client',1,'')`, "listening records are saved for this work"},
			{"offline copy", `INSERT INTO libraries(id,name,root_path,created_at,updated_at) VALUES('offline','Offline','/nonexistent-preen-library','',''); INSERT INTO work_copies VALUES('copy','RJ00000001','offline','work','')`, "a local copy is bound to this work"},
		} {
			t.Run(seam+"/"+tc.name, func(t *testing.T) {
				repo := deletionRepository(t)
				deletionSQL(t, repo, tc.query)
				notifications := 0
				repo.OnCatalogChanged = func() { notifications++ }
				remove := repo.DeleteWork
				if seam == "service" {
					remove = works.NewService(repo, nil, nil, nil, "").Delete
				}
				refusal := remove(deletionWork)
				if refusal == nil || refusal.Code != works.CodeWorkInUse || len(refusal.Details) != 1 ||
					!slices.Contains(refusal.Details, tc.detail) {
					t.Fatalf("delete refusal = %+v, want WORK_IN_USE with %q", refusal, tc.detail)
				}
				if _, found, err := repo.GetWork(deletionWork); err != nil || !found {
					t.Fatalf("protected work: found=%v err=%v", found, err)
				}
				if notifications != 0 {
					t.Fatal("refused delete notified catalog change")
				}
			})
		}
	}
}

func TestWorkDeletionAllowsEmptyUserDataAndPreservesSettings(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(t, repo, `INSERT INTO user_tags VALUES('unused','keep')`)
	if _, err := repo.PatchUserData(deletionWork, works.UserDataPatch{
		Rating:   works.NullableInt{Set: true, Value: new(8)},
		Note:     works.Optional[string]{Set: true, Value: "keep"},
		Favorite: works.Optional[bool]{Set: true, Value: true},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PatchUserData(deletionWork, works.UserDataPatch{
		Rating:   works.NullableInt{Set: true},
		Note:     works.Optional[string]{Set: true, Value: ""},
		Favorite: works.Optional[bool]{Set: true, Value: false},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddTag("keep classifier"); err != nil {
		t.Fatal(err)
	}
	notifications := 0
	repo.OnCatalogChanged = func() { notifications++ }
	service := works.NewService(repo, nil, nil, nil, "")
	if refusal := service.Delete(deletionWork); refusal != nil {
		t.Fatal(refusal)
	}
	if notifications != 1 {
		t.Fatalf("notifications=%d, want 1", notifications)
	}
	if _, found, err := repo.GetWork(deletionWork); err != nil || found {
		t.Fatalf("deleted work: found=%v err=%v", found, err)
	}
	tags, err := repo.ListUserTags()
	if err != nil || len(tags) != 1 {
		t.Fatalf("user vocabulary=%v err=%v", tags, err)
	}
	classifiers, err := repo.Tags()
	if err != nil || len(classifiers) != 1 {
		t.Fatalf("classifier settings=%v err=%v", classifiers, err)
	}
	if refusal := service.Delete(deletionWork); refusal == nil || refusal.Code != works.CodeWorkNotFound {
		t.Fatalf("second delete=%v", refusal)
	}
	if notifications != 1 {
		t.Fatal("missing work delete notified catalog change")
	}
}

func TestWorkDeletionRollbackDoesNotNotify(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(
		t,
		repo,
		`CREATE TRIGGER refuse_work_delete BEFORE DELETE ON works BEGIN SELECT RAISE(ABORT,'injected delete failure'); END`,
	)
	notifications := 0
	repo.OnCatalogChanged = func() { notifications++ }
	if refusal := repo.DeleteWork(deletionWork); refusal == nil || refusal.Kind != works.KindInternal {
		t.Fatalf("refusal=%v", refusal)
	}
	if _, found, err := repo.GetWork(deletionWork); err != nil || !found {
		t.Fatalf("rolled back work: found=%v err=%v", found, err)
	}
	if notifications != 0 {
		t.Fatal("rolled back delete notified catalog change")
	}
}

func assertMissingWork(t *testing.T, err error) {
	t.Helper()
	var refusal *works.Error
	if !errors.As(err, &refusal) || refusal.Code != works.CodeWorkNotFound {
		t.Fatalf("edit error=%v, want WORK_NOT_FOUND", err)
	}
}

func TestWorkDeletionAndEditsRespectCommitOrder(t *testing.T) {
	for _, seam := range []string{"repository", "service"} {
		for _, kind := range []string{"rating", "note", "favorite", "tags", "collection", "listening"} {
			for _, editFirst := range []bool{true, false} {
				t.Run(
					seam+"/"+kind+"/editFirst="+map[bool]string{true: "true", false: "false"}[editFirst],
					func(t *testing.T) {
						repo := deletionRepository(t)
						service := works.NewService(repo, nil, nil, nil, "")
						tag, _, err := repo.CreateUserTag("tag")
						if err != nil {
							t.Fatal(err)
						}
						edit := func() error { return editDeletionWork(repo, seam, kind, tag.ID) }
						if editFirst {
							if err := edit(); err != nil {
								t.Fatal(err)
							}
							if refusal := service.Delete(
								deletionWork,
							); refusal == nil ||
								refusal.Code != works.CodeWorkInUse {
								t.Fatalf("delete after edit=%v", refusal)
							}
						} else {
							if refusal := service.Delete(deletionWork); refusal != nil {
								t.Fatal(refusal)
							}
							assertMissingWork(t, edit())
						}
					},
				)
			}
		}
	}
}

func editDeletionWork(repo *sqlite.Repository, seam, kind, tagID string) error {
	service := works.NewService(repo, nil, nil, nil, "")
	req := works.UserDataRequest{}
	switch kind {
	case "rating":
		req.Rating = works.NullableInt{Set: true, Value: new(8)}
	case "note":
		req.Note = new(" ")
	case "favorite":
		req.Favorite = new(true)
	case "tags":
		if seam == "service" {
			_, e := service.ReplaceUserTags(deletionWork, works.UserTagsRequest{TagIDs: []string{tagID}})
			if e != nil {
				return e
			}
			return nil
		}
		_, e := repo.ReplaceUserTags(deletionWork, []string{tagID})
		return e
	case "collection":
		if seam == "service" {
			_, e := collections.NewService(repo).
				Append(context.Background(), collections.FavoritesID, []string{string(deletionWork)})
			if e != nil {
				return e
			}
			return nil
		}
		_, e := repo.AppendCollectionWorks(
			context.Background(),
			collections.FavoritesID,
			[]works.WorkCode{deletionWork},
			time.Now(),
		)
		return e
	case "listening":
		listening := works.ListeningRequest{TrackKey: "track", TrackCount: 1, Client: "client", Seq: 1}
		if seam == "service" {
			_, e := service.PutListening(deletionWork, listening)
			if e != nil {
				return e
			}
			return nil
		}
		_, e := repo.PutListening(deletionWork, listening, time.Now())
		return e
	}
	if seam == "service" {
		_, e := service.PatchUserData(deletionWork, req)
		if e != nil {
			return e
		}
		return nil
	}
	patch, invalid := req.Patch()
	if invalid != nil {
		return invalid
	}
	_, e := repo.PatchUserData(deletionWork, patch, time.Now())
	return e
}

func TestWorkDeletionMakesSubsequentRemovalsMissing(t *testing.T) {
	repo := deletionRepository(t)
	if err := repo.ClearListening(deletionWork); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveCollectionWork(
		context.Background(),
		collections.FavoritesID,
		deletionWork,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	if refusal := repo.DeleteWork(deletionWork); refusal != nil {
		t.Fatal(refusal)
	}
	assertMissingWork(t, repo.ClearListening(deletionWork))
	assertMissingWork(
		t,
		repo.RemoveCollectionWork(context.Background(), collections.FavoritesID, deletionWork, time.Now()),
	)
	assertMissingWork(
		t,
		repo.MoveCollectionWork(context.Background(), collections.FavoritesID, deletionWork, nil, time.Now()),
	)
}

func TestWorkDeletionRacesWithPersonalEdits(t *testing.T) {
	for _, kind := range []string{"rating", "note", "favorite", "tags", "collection", "listening"} {
		t.Run(kind, func(t *testing.T) {
			repo := deletionRepository(t)
			tag, _, err := repo.CreateUserTag("tag")
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			editResult := make(chan error, 1)
			deleteResult := make(chan *works.Error, 1)
			go func() { <-start; editResult <- editDeletionWork(repo, "service", kind, tag.ID) }()
			go func() { <-start; deleteResult <- works.NewService(repo, nil, nil, nil, "").Delete(deletionWork) }()
			close(start)
			editErr, refusal := <-editResult, <-deleteResult
			if editErr == nil {
				if refusal == nil || refusal.Code != works.CodeWorkInUse {
					t.Fatalf("saved personal data but delete=%v", refusal)
				}
			} else {
				assertMissingWork(t, editErr)
				if refusal != nil {
					t.Fatalf("delete=%v, edit=%v", refusal, editErr)
				}
			}
		})
	}
}

func TestWorkEditStorageFailureIsNotMissingWork(t *testing.T) {
	repo := deletionRepository(t)
	deletionSQL(
		t,
		repo,
		`CREATE TRIGGER refuse_personal_data BEFORE INSERT ON work_user_data BEGIN SELECT RAISE(ABORT,'FOREIGN KEY constraint failed'); END`,
	)
	_, refusal := works.NewService(repo, nil, nil, nil, "").
		PatchUserData(deletionWork, works.UserDataRequest{Note: new("keep")})
	if refusal == nil || refusal.Kind != works.KindInternal {
		t.Fatalf("storage failure=%v, want internal", refusal)
	}
}
