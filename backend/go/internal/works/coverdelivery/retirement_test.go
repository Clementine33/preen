package coverdelivery_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// A real plan replacement runs inside the same guard used by process assembly.
func replacement(t *testing.T, svc *coverdelivery.Service, repo *sqlite.Repository) func() error {
	t.Helper()
	if _, err := repo.DB().
		Exec(`INSERT INTO workset_operations(workset_id,operation_type,current_revision_id) VALUES('set','metadata_refresh','plan')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateGeneration(&workset.PlanGeneration{
		GenerationID: "replacement", WorksetID: "set", OperationType: "metadata_refresh", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.NextQueuedGeneration(nil); err != nil {
		t.Fatal(err)
	}
	repo.PlanRetirementGuard = func(write func() error) error { return svc.WithPublication(t.Context(), write) }
	return func() error {
		return repo.PersistOperationRevision(t.Context(),
			"replacement",
			"set",
			"metadata_refresh",
			time.Now(),
			workset.OperationRevisionPersist{
				PlanID: "replacement-plan", RootPath: "/music", SnapshotToken: "replacement-snapshot",
			},
		)
	}
}

func TestRetirementBlocksClaimedCoverAtPublicationRecheck(t *testing.T) {
	testRetirementBlocksClaimedCover(t, replacement)
}

func TestRecordRetirementBlocksClaimedCoverAtPublicationRecheck(t *testing.T) {
	testRetirementBlocksClaimedCover(t, recordReplacement)
}

func testRetirementBlocksClaimedCover(
	t *testing.T,
	replace func(*testing.T, *coverdelivery.Service, *sqlite.Repository) func() error,
) {
	t.Helper()
	_, repo, dl, req, folders := deliveryFixture(t)
	// Retire at file admission, just before deliver acquires publication and
	// rechecks current demand. No active-download cancellation is needed to stop it.
	var retire func() error
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: repo, Downloader: dl, Files: filesystem.CoverFiles{},
		Acquire: func() (func(), error) {
			err := retire()
			return func() {}, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	retire = sync.OnceValue(replace(t, svc, repo))
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, folder := range folders {
		if _, err = os.Stat(filepath.Join(folder, "cover.png")); !os.IsNotExist(err) {
			t.Fatalf("retired demand published: %v", err)
		}
	}
}

type heldPublication struct {
	filesystem.CoverFiles

	started chan struct{}
	release chan struct{}
	first   bool
}

func (f *heldPublication) Publish(
	ctx context.Context,
	target coverdelivery.Target,
	data []byte,
	digest string,
	refresh bool,
) (coverdelivery.Publication, error) {
	if !f.first {
		f.first = true
		close(f.started)
		<-f.release
	}
	return f.CoverFiles.Publish(ctx, target, data, digest, refresh)
}

func TestRetirementWaitsForPublicationThatPassedDemandRecheck(t *testing.T) {
	testRetirementWaitsForPublication(t, replacement)
}

func TestRecordRetirementWaitsForPublicationThatPassedDemandRecheck(t *testing.T) {
	testRetirementWaitsForPublication(t, recordReplacement)
}

func testRetirementWaitsForPublication(
	t *testing.T,
	replace func(*testing.T, *coverdelivery.Service, *sqlite.Repository) func() error,
) {
	t.Helper()
	_, repo, dl, req, _ := deliveryFixture(t)
	files := &heldPublication{started: make(chan struct{}), release: make(chan struct{})}
	svc, err := coverdelivery.New(coverdelivery.Dependencies{
		Store: repo, Downloader: dl, Files: files, Acquire: func() (func(), error) { return func() {}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	retire := replace(t, svc, repo)
	attempted := make(chan struct{})
	repo.PlanRetirementGuard = func(write func() error) error {
		close(attempted)
		return svc.WithPublication(t.Context(), write)
	}
	if err = svc.Enqueue(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	publicationDone := make(chan error, 1)
	go func() { _, processErr := svc.ProcessNext(t.Context()); publicationDone <- processErr }()
	<-files.started // demand has returned true and publication still owns its lock
	retirementDone := make(chan error, 1)
	go func() { retirementDone <- retire() }()
	<-attempted
	select {
	case err = <-retirementDone:
		t.Fatalf("retirement overtook protected publication: %v", err)
	case <-time.After(50 * time.Millisecond): // Mutex waits cannot advance synctest virtual time.
	}
	// The guard must wait before BEGIN: an unrelated writer remains free.
	if _, err = repo.DB().Exec(`UPDATE worksets SET title='concurrent edit' WHERE id='set'`); err != nil {
		t.Fatalf("retirement held database lock while waiting for publication: %v", err)
	}
	close(files.release)
	if err = <-retirementDone; err != nil {
		t.Fatal(err)
	}
	if err = <-publicationDone; err != nil {
		t.Fatal(err)
	}
	var published int
	if err = repo.DB().
		QueryRow(`SELECT COUNT(*) FROM cover_deliveries WHERE status='succeeded'`).
		Scan(&published); err != nil ||
		published < 1 {
		t.Fatalf("publication preceding retirement was lost: %d, %v", published, err)
	}
	if job, claimErr := repo.ClaimCover(t.Context()); claimErr != nil || job != nil {
		t.Fatalf("retired demand remained claimable: %+v, %v", job, claimErr)
	}
}

func recordReplacement(t *testing.T, svc *coverdelivery.Service, repo *sqlite.Repository) func() error {
	t.Helper()
	if _, err := repo.DB().Exec(`INSERT INTO libraries(id,name,root_path) VALUES('replacement-lib','Library','/music');
 UPDATE worksets SET library_id='replacement-lib' WHERE id='set';
 UPDATE plan_executions SET status='succeeded' WHERE execution_id='exec'`); err != nil {
		t.Fatal(err)
	}
	repo.PlanRetirementGuard = func(write func() error) error { return svc.WithPublication(t.Context(), write) }
	return func() error {
		now := time.Now()
		return repo.ReplaceCurrentWorkset(
			&workset.Workset{
				ID:        "replacement-set",
				LibraryID: "replacement-lib",
				Version:   1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			nil,
			nil,
			nil,
			"set",
		)
	}
}
