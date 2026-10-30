package sqlite //nolint:testpackage // white-box tests observe the unexported import lock

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Clementine33/preen/backend/internal/works"
)

func TestImportReleasesTheLockAfterFailures(t *testing.T) {
	repo := newTestRepository(t)
	first := seedWork(t, repo, "RJ00000001")
	missing := code(t, "RJ00000099")

	_, err := repo.ImportMetadata(&works.ValidatedImport{
		Target: missing, Core: coreGroup(t, works.SourceDLsite, "x"),
	}, mustTime(t, testStamp))
	if err == nil {
		t.Fatal("import into an unknown work was accepted")
	}

	// A structural refusal inside the transaction.
	self := first
	_, err = repo.ImportMetadata(&works.ValidatedImport{
		Target: first,
		Relations: &works.RelationsGroup{
			Source: works.SourceDLsite, FetchedAt: mustTime(t, testStamp),
			Role: works.RoleChild, OriginalRJ: &self,
		},
	}, mustTime(t, testStamp))
	if err == nil {
		t.Fatal("a self-original was accepted")
	}

	// The lock was released both times: the next import proceeds.
	importGroups(t, repo, first, &works.ValidatedImport{
		Target: first, Core: coreGroup(t, works.SourceDLsite, "after failures"),
	})
}

func TestConcurrentImportsMatchSerialApplication(t *testing.T) {
	const n = 24
	build := func(t *testing.T, repo *Repository) []*works.ValidatedImport {
		t.Helper()
		parent := seedWork(t, repo, "RJ00000100")
		ins := []*works.ValidatedImport{{
			Target: parent,
			Core:   coreGroup(t, works.SourceDLsite, "parent"),
			Relations: relationsGroup(t, works.RoleParent, nil,
				"RJ00000101", "RJ00000102", "RJ00000103"),
		}}
		for i := range n {
			raw := fmt.Sprintf("RJ%08d", 101+i)
			id := seedWork(t, repo, raw)
			in := &works.ValidatedImport{
				Target: id, Core: coreGroup(t, works.SourceDLsite, "title "+raw),
			}
			if i < 3 { // shared target: declared by the parent above
				in.Relations = relationsGroup(t, works.RoleChild, new("RJ00000100"))
			}
			ins = append(ins, in)
		}
		return ins
	}
	snapshot := func(t *testing.T, repo *Repository) string {
		t.Helper()
		var out strings.Builder
		for _, q := range []string{
			"SELECT id, title FROM works ORDER BY id",
			"SELECT work_id, source, fetched_at FROM work_source_state ORDER BY work_id, data_group",
			"SELECT subject_rj, parent_rj FROM relation_reports ORDER BY subject_rj",
			"SELECT work_id FROM work_translations ORDER BY work_id",
		} {
			rows, err := repo.DB().Query(q)
			if err != nil {
				t.Fatal(err)
			}
			cols, _ := rows.Columns()
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					t.Fatal(err)
				}
				fmt.Fprintln(&out, vals...)
			}
			rows.Close()
		}
		return out.String()
	}

	serial := newTestRepository(t)
	for _, in := range build(t, serial) {
		importGroups(t, serial, in.Target, in)
	}

	concurrent := newTestRepository(t)
	var wg sync.WaitGroup
	errs := make(chan error, n+1)
	for _, in := range build(t, concurrent) {
		wg.Go(func() {
			_, err := concurrent.ImportMetadata(in, mustTime(t, testStamp))
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent import: %v", err)
		}
	}
	if got, want := snapshot(t, concurrent), snapshot(t, serial); got != want {
		t.Errorf("concurrent state differs from serial:\n%s\nvs\n%s", got, want)
	}
}

func TestCatalogChangedRunsAfterTheImportLockIsReleased(t *testing.T) {
	repo := newTestRepository(t)
	id := seedWork(t, repo, "RJ00000001")
	var calls int
	var heldDuring bool
	repo.OnCatalogChanged = func() {
		calls++
		if repo.importMu.TryLock() {
			repo.importMu.Unlock()
		} else {
			heldDuring = true
		}
	}
	importGroups(t, repo, id, &works.ValidatedImport{
		Target: id, Core: coreGroup(t, works.SourceDLsite, "t"),
	})
	if calls != 1 || heldDuring {
		t.Errorf("calls=%d heldDuring=%v, want one call with the lock released", calls, heldDuring)
	}
}
