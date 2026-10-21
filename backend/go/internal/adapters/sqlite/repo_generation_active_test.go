package sqlite_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func TestGenerationActiveUniquenessInBothDatabaseModes(t *testing.T) {
	for name, open := range map[string]func(string) (*sqlite.Repository, error){
		"development": sqlite.NewDevelopmentRepository,
		"release":     sqlite.NewRepository,
	} {
		t.Run(name, func(t *testing.T) {
			repo, err := open(filepath.Join(t.TempDir(), "cache.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			_, err = repo.DB().Exec("INSERT INTO worksets (id) VALUES ('ws-a'), ('ws-b')")
			if err != nil {
				t.Fatal(err)
			}
			insert := func(id, ws, op, key string) error {
				return repo.CreateGeneration(&workset.PlanGeneration{
					GenerationID: id, WorksetID: ws, OperationType: op,
					IdempotencyKey: key, CreatedAt: time.Now(),
				})
			}
			err = insert("a", "ws-a", "conversion", "key-a")
			if err != nil {
				t.Fatal(err)
			}
			err = insert("b", "ws-a", "conversion", "key-b")
			if !errors.Is(err, workset.ErrGenerationActiveConflict) {
				t.Fatalf("different-key active collision = %v", err)
			}
			err = insert("same", "ws-a", "conversion", "key-a")
			if !errors.Is(err, workset.ErrGenerationIdemConflict) {
				t.Fatalf("same-key collision = %v", err)
			}
			err = insert("other-op", "ws-a", "rename", "key-b")
			if err != nil {
				t.Fatalf("other operation: %v", err)
			}
			err = insert("other-ws", "ws-b", "conversion", "key-b")
			if err != nil {
				t.Fatalf("other workset: %v", err)
			}
			_, err = repo.DB().Exec(
				"UPDATE plan_generations SET status = 'failed' WHERE generation_id = 'a'",
			)
			if err != nil {
				t.Fatal(err)
			}
			err = insert("after-failure", "ws-a", "conversion", "key-a")
			if err != nil {
				t.Fatalf("terminal session kept active slot: %v", err)
			}
		})
	}
}
