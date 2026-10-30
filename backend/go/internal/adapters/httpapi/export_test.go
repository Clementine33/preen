package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// MemberReviewHTTPFixture provides authenticated GET requests against a real
// service with a published single-member plan, plus its record and member IDs.
func MemberReviewHTTPFixture(t *testing.T) (get func(string) *httptest.ResponseRecorder, worksetID, memberID string) {
	t.Helper()
	h, repo := newWorksetServer(t)
	libID := seedLibrary(t, repo)
	seedMember(t, repo, "albumA")
	worksetID = createRecord(t, h, libID, "member-review-http")
	svc := workset.NewService(repo, 1, 1, []workset.Task{&conversion.Task{}}, nil, nil)
	seedRevision(t, repo, svc, libID, worksetID, "member-plan")
	members, err := repo.ListWorksetMembers(worksetID)
	if err != nil {
		t.Fatal(err)
	}
	return func(path string) *httptest.ResponseRecorder {
		t.Helper()
		return req(t, h, http.MethodGet, path, testToken, nil)
	}, worksetID, members[0].MemberID
}
