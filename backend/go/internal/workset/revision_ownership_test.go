package workset_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/testutil/inventoryfixture"
)

// TestRevisionComponentRootOwnership reads each member's complete frozen inputs
// through member review and checks stable, disjoint component ownership.
func TestRevisionComponentRootOwnership(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA", "albumB")

	// albumA holds a wav+mp3 pair (one matched component), albumB a lone wav
	// (one unmatched component). Parent directories are required for grouping.
	f.insertNestedDir("/music/albumA/wav")
	f.insertNestedDir("/music/albumA/mp3")
	f.insertAudioEntry("/music/albumA/wav/test1.wav", "/music/albumA", 1024, 1000)
	f.insertAudioEntry("/music/albumA/mp3/test1.mp3", "/music/albumA", 2048, 1000)
	f.insertNestedDir("/music/albumB/wav")
	f.insertAudioEntry("/music/albumB/wav/track.wav", "/music/albumB", 4096, 1000)

	ws := f.createCurrent("双专辑", ids...)
	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "completed" || gen.RevisionID == "" {
		t.Fatalf("generation did not complete: %+v", gen)
	}

	seen := map[string]bool{}
	for _, member := range ws.Members {
		review, err := f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, member.MemberID)
		if err != nil {
			t.Fatal(err)
		}
		if len(review.Roots) != 1 || review.Roots[0].RootPath != member.FolderPath || len(review.Plan.Units) == 0 {
			t.Fatalf("wrong member inputs: %+v", review)
		}
		for _, raw := range review.Plan.Units {
			var component struct {
				ComponentID string `json:"component_id"`
			}
			if decodeErr := json.Unmarshal(raw, &component); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if component.ComponentID == "" || seen[component.ComponentID] {
				t.Fatalf("invalid ownership: %s", raw)
			}
			seen[component.ComponentID] = true
			other := "/music/albumA"
			if member.FolderPath == other {
				other = "/music/albumB"
			}
			if strings.Contains(string(raw), other) {
				t.Fatalf("foreign component in member review: %s", raw)
			}
		}
		again, err := f.svc.GetMemberReview(f.ctx, ws.WorksetID, "conversion", gen.RevisionID, member.MemberID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(review, again) {
			t.Fatal("frozen member review changed on re-read")
		}
	}
}

// insertNestedDir inserts a directory row for the planner's component
// grouping.
func (f *fixture) insertNestedDir(path string) {
	f.t.Helper()
	now := time.Now().Unix()
	inventoryfixture.SeedInventory(
		f.t,
		f.repo,
		inventoryfixture.Row{Path: path, RootPath: "/music", IsDir: true, CreatedAt: now},
	)
}
