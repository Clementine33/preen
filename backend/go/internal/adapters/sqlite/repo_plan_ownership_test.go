package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
)

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s: got %d, want %d", query, got, want)
	}
}

func seedOwnership(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		`INSERT INTO worksets(id) VALUES('ws-1'),('ws-2')`,
		`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('p-1','/music','s','ws-1'),('p-2','/music','s','ws-2')`,
		`INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('e-1','ws-1','p-1'),('e-2','ws-2','p-2')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO plans(plan_id,root_path,snapshot_token) VALUES('missing-owner','/music','s')`,
		`INSERT INTO plans(plan_id,root_path,snapshot_token,workset_id) VALUES('orphan','/music','s','missing')`,
		`INSERT INTO plan_executions(execution_id,workset_id,plan_id) VALUES('orphan','ws-1','missing')`,
	} {
		if _, err := db.Exec(stmt); err == nil {
			t.Fatalf("accepted orphan: %s", stmt)
		}
	}
}

func assertCascadeIndex(t *testing.T, db *sql.DB, query, index string) {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var detail strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var step string
		if err := rows.Scan(&id, &parent, &unused, &step); err != nil {
			t.Fatal(err)
		}
		detail.WriteString(step)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail.String(), index) {
		t.Fatalf("cascade lookup lacks %s: %s", index, detail.String())
	}
}

func TestPlanOwnershipAndCoverDemandCascade(t *testing.T) {
	for name, open := range map[string]func(string) (*sqlite.Repository, error){
		"development": sqlite.NewDevelopmentRepository,
		"release":     sqlite.NewRepository,
	} {
		t.Run(name, func(t *testing.T) {
			repo, err := open(t.TempDir() + "/cache.db")
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			db := repo.DB()
			seedOwnership(t, db)
			for _, query := range []struct{ sql, index string }{
				{`EXPLAIN QUERY PLAN DELETE FROM worksets WHERE id='ws-1'`, "idx_plans_workset"},
				{`EXPLAIN QUERY PLAN DELETE FROM plans WHERE plan_id='p-1'`, "idx_plan_executions_plan_unique"},
			} {
				assertCascadeIndex(t, db, query.sql, query.index)
			}
			ctx := context.Background()
			target := coverdelivery.Target{CopyID: "copy-1", WorkCode: "RJ1"}
			for _, execution := range []string{"e-1", "e-2"} {
				if err := repo.EnqueueCovers(ctx, coverdelivery.Request{
					ExecutionID: execution, WorkCode: "RJ1", URL: "https://example.com/cover",
					Targets: []coverdelivery.Target{target},
				}); err != nil {
					t.Fatal(err)
				}
			}
			var job coverdelivery.Job
			if err := db.QueryRow(`SELECT id,work_code,version FROM cover_downloads`).
				Scan(&job.ID, &job.WorkCode, &job.Version); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`DELETE FROM plans WHERE plan_id='p-1'`); err != nil {
				t.Fatal(err)
			}
			assertCount(t, db, `SELECT COUNT(*) FROM plan_executions WHERE execution_id='e-1'`, 0)
			assertCount(t, db, `SELECT COUNT(*) FROM cover_deliveries WHERE execution_id='e-1'`, 0)
			assertCount(t, db, `SELECT COUNT(*) FROM cover_deliveries WHERE execution_id='e-2'`, 1)
			if current, err := repo.CoverCurrent(ctx, job); err != nil || !current {
				t.Fatalf("shared download stopped: current=%v err=%v", current, err)
			}
			if _, err := db.Exec(`DELETE FROM worksets WHERE id='ws-2'`); err != nil {
				t.Fatal(err)
			}
			assertCount(t, db, `SELECT COUNT(*) FROM plans WHERE plan_id='p-2'`, 0)
			assertCount(t, db, `SELECT COUNT(*) FROM cover_deliveries`, 0)
			assertCount(t, db, `SELECT COUNT(*) FROM cover_downloads`, 1)
			if current, err := repo.CoverCurrent(ctx, job); err != nil || current {
				t.Fatalf("retired demand republished: current=%v err=%v", current, err)
			}
		})
	}
}
