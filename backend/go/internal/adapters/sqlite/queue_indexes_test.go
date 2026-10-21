package sqlite_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
)

func TestSessionQueuesUseTimeOrderedIndexes(t *testing.T) {
	for mode, open := range map[string]func(string) (*sqlite.Repository, error){
		"development": sqlite.NewDevelopmentRepository,
		"release":     sqlite.NewRepository,
	} {
		t.Run(mode, func(t *testing.T) {
			repo, err := open(filepath.Join(t.TempDir(), "queue.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			checkSessionQueuePlans(t, repo.DB())
			seedSessionQueues(t, repo.DB())
			checkSessionQueueClaims(t, repo)
		})
	}
}

func checkSessionQueuePlans(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, tc := range []struct {
		table, index, query string
	}{
		{"plan_generations", "idx_plan_generations_queue", "SELECT generation_id FROM plan_generations WHERE status = 'queued' AND operation_type IN (SELECT value FROM json_each(?)) ORDER BY julianday(created_at), generation_id LIMIT 1"},
		{"plan_executions", "idx_plan_executions_queue", "SELECT execution_id FROM plan_executions WHERE status = 'queued' ORDER BY julianday(created_at), execution_id LIMIT 1"},
	} {
		args := []any{}
		if tc.table == "plan_generations" {
			args = append(args, `["conversion"]`)
		}
		rows, queryErr := db.Query("EXPLAIN QUERY PLAN "+tc.query, args...)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		var details []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if scanErr := rows.Scan(&id, &parent, &unused, &detail); scanErr != nil {
				t.Fatal(scanErr)
			}
			details = append(details, detail)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		plan := strings.Join(details, "\n")
		if !strings.Contains(plan, tc.index) || strings.Contains(plan, "USE TEMP B-TREE FOR ORDER BY") {
			t.Fatalf("%s queue plan: %s", tc.table, plan)
		}
	}
}

func checkSessionQueueClaims(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	for _, want := range []string{"g-early", "g-a", "g-b"} {
		got, claimErr := repo.NextQueuedGeneration([]string{"conversion"})
		if claimErr != nil || got == nil || got.GenerationID != want {
			t.Fatalf("generation claim = %+v, %v; want %s", got, claimErr, want)
		}
	}
	if got, claimErr := repo.NextQueuedGeneration([]string{"conversion"}); claimErr != nil || got != nil {
		t.Fatalf("remaining conversion generation = %+v, %v", got, claimErr)
	}
	if got, claimErr := repo.NextQueuedGeneration(nil); claimErr != nil || got == nil ||
		got.GenerationID != "g-other" {
		t.Fatalf("other generation = %+v, %v", got, claimErr)
	}
	for _, want := range []string{"e-early", "e-a", "e-b"} {
		got, claimErr := repo.NextQueuedExecution()
		if claimErr != nil || got == nil || got.ExecutionID != want {
			t.Fatalf("execution claim = %+v, %v; want %s", got, claimErr, want)
		}
	}
	if got, claimErr := repo.NextQueuedExecution(); claimErr != nil || got != nil {
		t.Fatalf("remaining execution = %+v, %v", got, claimErr)
	}
}

func seedSessionQueues(t *testing.T, db *sql.DB) {
	t.Helper()
	for i, row := range []struct{ id, status, kind, created string }{
		{"early", "queued", "conversion", "2025-12-31T23:30:00Z"},
		{"a", "queued", "conversion", "2026-01-01T08:00:00+08:00"},
		{"b", "queued", "conversion", "2025-12-31T23:00:00-01:00"},
		{"skip", "canceled", "conversion", "2025-12-30T00:00:00Z"},
		{"other", "queued", "rename", "2025-12-31T22:00:00Z"},
	} {
		lib, ws, plan := fmt.Sprintf("lib-%d", i), fmt.Sprintf("ws-%d", i), fmt.Sprintf("plan-%d", i)
		if _, err := db.Exec(
			"INSERT INTO libraries (id, name, root_path, root_path_key) VALUES (?, ?, ?, ?)",
			lib, lib, "/music/"+lib, "/music/"+lib,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			"INSERT INTO worksets (id, library_id, root_path, root_path_key) VALUES (?, ?, ?, ?)",
			ws, lib, "/music/"+lib, "/music/"+lib,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			"INSERT INTO plans (plan_id, root_path, snapshot_token, workset_id) VALUES (?, '/music', 'snapshot', ?)",
			plan, ws,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			`INSERT INTO plan_generations
			 (generation_id, workset_id, operation_type, status, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			"g-"+row.id, ws, row.kind, row.status, row.created,
		); err != nil {
			t.Fatal(err)
		}
		if row.kind == "rename" {
			continue
		}
		if _, err := db.Exec(
			`INSERT INTO plan_executions
			 (execution_id, workset_id, operation_type, plan_id, status, created_at)
			 VALUES (?, ?, 'conversion', ?, ?, ?)`,
			"e-"+row.id, ws, plan, row.status, row.created,
		); err != nil {
			t.Fatal(err)
		}
	}
}
