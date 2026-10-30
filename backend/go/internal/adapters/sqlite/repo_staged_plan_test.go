package sqlite_test

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/workset"
)

func stagedGeneration(t *testing.T, repo *sqlite.Repository, id, kind string) {
	t.Helper()
	if err := repo.CreateGeneration(
		&workset.PlanGeneration{GenerationID: id, WorksetID: "set", OperationType: kind, CreatedAt: time.Now()},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NextQueuedGeneration(nil); err != nil {
		t.Fatal(err)
	}
}

func stagedPayload(id string) workset.OperationRevisionPersist {
	p := workset.OperationRevisionPersist{
		PlanID:        id,
		RootPath:      "/music",
		SnapshotToken: id,
		SummaryReason: "blocked",
		Counts:        workset.RevisionCounts{Members: 251, Changed: 1, UnmetTargets: 2, Blocked: 3, Unchanged: 4},
		Steps:         []workset.PlanStepRecord{{StepIndex: 0, StepType: "test"}},
	}
	for i := range 251 {
		p.Roots = append(p.Roots, workset.PlanRootRecord{RootIndex: i, RootPath: fmt.Sprintf("/music/%d", i)})
		p.Components = append(
			p.Components,
			workset.PlanComponentRecord{ComponentIndex: i, ComponentID: strconv.Itoa(i), RootIndex: i},
		)
		p.Conclusions = append(p.Conclusions, workset.MemberConclusion{MemberID: "m" + strconv.Itoa(i)})
	}
	return p
}

func TestStagingIsHiddenAndCleanupCannotDeleteRunningGeneration(t *testing.T) {
	for _, kind := range []string{"conversion", "rename", "metadata_refresh"} {
		t.Run(kind, func(t *testing.T) {
			repo, _ := retiredPlanFixture(t, kind)
			stagedGeneration(t, repo, "second", kind)
			repo.PlanRetirementGuard = func(write func() error) error {
				if _, err := repo.GetPlanDetail("new"); !errors.Is(err, workset.ErrPlanNotFound) {
					t.Fatalf("staged plan visible: %v", err)
				}
				if roots, err := repo.GetPlanRoots("new"); err != nil || len(roots) != 0 {
					t.Fatalf("staged roots visible: %v %v", roots, err)
				}
				if _, err := repo.GetOperationRevision(
					"set",
					kind,
					"new",
				); !errors.Is(
					err,
					workset.ErrRevisionNotFound,
				) {
					t.Fatalf("staged revision visible: %v", err)
				}
				if _, err := repo.GetPlanDetail("old"); err != nil {
					t.Fatal(err)
				}
				if n, err := repo.DeleteRetiredPlanBatch(t.Context(), 1000); err != nil || n != 0 {
					t.Fatalf("cleaned live staging: %d %v", n, err)
				}
				var generation string
				if err := repo.DB().
					QueryRow(`SELECT generation_id FROM plans WHERE plan_id='new' AND status='staging'`).
					Scan(&generation); err != nil ||
					generation != "second" {
					t.Fatalf("ownership: %s %v", generation, err)
				}
				return write()
			}
			if err := repo.PersistOperationRevision(
				t.Context(),
				"second",
				"set",
				kind,
				time.Now(),
				stagedPayload("new"),
			); err != nil {
				t.Fatal(err)
			}
			if detail, err := repo.GetPlanDetail(
				"new",
			); err != nil || len(detail.Components) != 251 ||
				len(detail.Roots) != 251 {
				t.Fatalf("published plan incomplete: %+v %v", detail, err)
			} else if detail.Plan.SummaryReason != "blocked" ||
				detail.Plan.Counts != (workset.RevisionCounts{Members: 251, Changed: 1, UnmetTargets: 2, Blocked: 3, Unchanged: 4}) {
				t.Fatalf("stored summary not round-tripped: %+v", detail.Plan)
			}
		})
	}
}

func TestCommittedCancellationRejectsNextStagingChunkAndPromotion(t *testing.T) {
	for _, at := range []string{"chunk", "promotion"} {
		t.Run(at, func(t *testing.T) {
			repo, _ := retiredPlanFixture(t, "conversion")
			stagedGeneration(t, repo, "second", "conversion")
			if at == "chunk" {
				if _, err := repo.DB().
					Exec(`CREATE TRIGGER cancel_staging AFTER INSERT ON plan_roots WHEN NEW.root_index=0 BEGIN UPDATE plan_generations SET cancel_requested=1 WHERE generation_id='second'; END`); err != nil {
					t.Fatal(err)
				}
			} else {
				repo.PlanRetirementGuard = func(write func() error) error {
					if err := repo.CancelGeneration("second"); err != nil {
						return err
					}
					return write()
				}
			}
			if err := repo.PersistOperationRevision(
				t.Context(),
				"second",
				"set",
				"conversion",
				time.Now(),
				stagedPayload("new"),
			); err == nil {
				t.Fatal("canceled generation published")
			}
			gen, err := repo.GetGeneration("second")
			if err != nil || gen.Status != "running" || !gen.CancelRequested || gen.RevisionID != "" {
				t.Fatalf("cancellation window: %+v %v", gen, err)
			}
			op, err := repo.GetOperation("set", "conversion")
			if err != nil || op.CurrentRevisionID != "old" {
				t.Fatalf("old plan replaced: %+v %v", op, err)
			}
			if at == "chunk" {
				var n int
				if err := repo.DB().
					QueryRow(`SELECT COUNT(*) FROM plan_roots WHERE plan_id='new'`).
					Scan(&n); err != nil || n == 0 ||
					n >= 251 {
					t.Fatalf("chunks not bounded: %d %v", n, err)
				}
			}
		})
	}
}

func TestCleanupBetweenCommittedStagingChunksPreservesCompleteRevision(t *testing.T) {
	repo, _ := retiredPlanFixture(t, "conversion")
	stagedGeneration(t, repo, "second", "conversion")
	paused, resume := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	if err := sqlite.PauseAfterFirstStagingChunk(repo, func() { close(paused); <-resume }); err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	var persistErr error
	go func() {
		defer close(finished)
		persistErr = repo.PersistOperationRevision(
			t.Context(),
			"second",
			"set",
			"conversion",
			time.Now(),
			stagedPayload("new"),
		)
	}()
	// Unblock and join persistence even when an assertion fails while it is paused.
	t.Cleanup(func() { release(); <-finished })
	select {
	case <-paused:
	case <-finished:
		t.Fatalf("persistence exited before checkpoint: %v", persistErr)
	case <-t.Context().Done():
		t.Fatal("persistence did not reach the committed chunk checkpoint")
	}
	var roots, components int
	if err := repo.DB().QueryRow(`SELECT (SELECT COUNT(*) FROM plan_roots WHERE plan_id='new'),
  (SELECT COUNT(*) FROM conversion_components WHERE plan_id='new')`).Scan(&roots, &components); err != nil || roots <= 0 || roots >= 251 || components != 0 {
		t.Fatalf("checkpoint must precede remaining payload: roots=%d components=%d err=%v", roots, components, err)
	}
	gen, err := repo.GetGeneration("second")
	if err != nil || gen.Status != workset.GenStatusRunning || gen.CancelRequested {
		t.Fatalf("staging owner is not active: %+v %v", gen, err)
	}
	if n, deleteErr := repo.DeleteRetiredPlanBatch(t.Context(), 1000); deleteErr != nil || n != 0 {
		t.Fatalf("cleanup deleted active staging between chunks: %d %v", n, deleteErr)
	}
	if _, err = repo.GetPlanDetail("new"); !errors.Is(err, workset.ErrPlanNotFound) {
		t.Fatalf("partial plan visible: %v", err)
	}
	if _, err = repo.GetPlanDetail("old"); err != nil {
		t.Fatalf("old plan unavailable during staging: %v", err)
	}
	release()
	<-finished
	if persistErr != nil {
		t.Fatal(persistErr)
	}
	detail, err := repo.GetPlanDetail("new")
	if err != nil || len(detail.Roots) != 251 || len(detail.Components) != 251 || len(detail.Steps) != 1 {
		t.Fatalf("published revision incomplete: %+v %v", detail, err)
	}
	op, err := repo.GetOperation("set", "conversion")
	if err != nil || op.CurrentRevisionID != "new" {
		t.Fatalf("complete revision not promoted: %+v %v", op, err)
	}
}
