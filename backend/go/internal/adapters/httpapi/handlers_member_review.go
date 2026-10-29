package httpapi

import (
	"net/http"
)

// memberReviewResponse carries frozen settings and only this member's inputs.
type memberReviewResponse struct {
	revisionMemberResponse

	PlanID string                   `json:"plan_id"`
	Roots  []rootValidationResponse `json:"roots"`
	Task   taskEnvelopeResponse     `json:"task"`
}

func (s *Server) revisionRoutes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle(
		"GET /api/v1/worksets/{id}/operations/{type}/revisions/{planId}",
		protect(http.HandlerFunc(s.getRevision)),
	)
	mux.Handle(
		"GET /api/v1/worksets/{id}/operations/{type}/revisions/{planId}/members/{memberId}",
		protect(http.HandlerFunc(s.getMemberReview)),
	)
}

// getMemberReview returns one member's frozen settings, input validation and
// complete task components; component ownership stays on the server.
func (s *Server) getMemberReview(w http.ResponseWriter, r *http.Request) {
	svc, err := s.worksetService()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "workset service not configured")
		return
	}
	review, err := svc.GetMemberReview(
		r.Context(),
		r.PathValue("id"),
		r.PathValue("type"),
		r.PathValue("planId"),
		r.PathValue("memberId"),
	)
	if err != nil {
		writeWorksetError(w, err)
		return
	}
	member := review.Member
	writeJSON(w, http.StatusOK, memberReviewResponse{
		MemberID: member.MemberID, MemberName: member.MemberName, FolderPath: member.FolderPath,
		Excluded: member.Excluded, Effective: member.Payload, Sources: member.Sources, Conclusion: member.Conclusion,
		PlanID: review.PlanID, Roots: toRoots(review.Roots), Task: toTaskEnvelope(review.Plan),
	})
}
