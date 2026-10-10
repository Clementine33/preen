package httpapi

import (
	"net/http"

	"github.com/onsei/organizer/backend/internal/works"
)

// ==================== detail ====================

// sourceEntityResponse is one circle, voice actor or source tag.
type sourceEntityResponse struct {
	ID       string  `json:"id"`
	Source   string  `json:"source"`
	SourceID *string `json:"source_id"`
	Name     string  `json:"name"`
}

func toSourceEntityResponses(entities []works.SourceEntity) []sourceEntityResponse {
	out := make([]sourceEntityResponse, 0, len(entities))
	for _, entity := range entities {
		out = append(out, sourceEntityResponse{
			ID:       entity.ID,
			Source:   string(entity.Source),
			SourceID: entity.SourceID,
			Name:     entity.Name,
		})
	}
	return out
}

// editionResponse is one declared language edition.
type editionResponse struct {
	VersionRootRJ string  `json:"version_root_rj"`
	Language      string  `json:"language"`
	Label         *string `json:"label"`
	EditionID     *string `json:"edition_id"`
	DisplayOrder  int     `json:"display_order"`
}

// relationResponse is the resolved relation: what the catalog derived from the
// evidence, not the evidence itself.
type relationResponse struct {
	Role       *string `json:"role"`
	Language   *string `json:"language"`
	OriginalRJ *string `json:"original_rj"`
	ParentRJ   *string `json:"parent_rj"`
}

func toRelationResponse(relation works.ResolvedRelation) relationResponse {
	out := relationResponse{Language: relation.Language}
	if relation.Role != nil {
		role := string(*relation.Role)
		out.Role = &role
	}
	if relation.OriginalID != nil {
		original := string(*relation.OriginalID)
		out.OriginalRJ = &original
	}
	if relation.ParentID != nil {
		parent := string(*relation.ParentID)
		out.ParentRJ = &parent
	}
	return out
}

// copyResponse is one directory bound to a work.
type copyResponse struct {
	CopyID    string `json:"copy_id"`
	LibraryID string `json:"library_id"`
	RelPath   string `json:"rel_path"`
}

// workDetailResponse is everything the detail endpoint answers with.
type workDetailResponse struct {
	workSummaryResponse

	SourceURL      *string                `json:"source_url"`
	CoverURL       *string                `json:"cover_url"`
	SampleCoverURL *string                `json:"sample_cover_url"`
	SeriesID       *string                `json:"series_id"`
	SeriesName     *string                `json:"series_name"`
	VoiceActors    []sourceEntityResponse `json:"voice_actors"`
	SourceTags     []sourceEntityResponse `json:"source_tags"`
	Note           string                 `json:"note"`
	Editions       []editionResponse      `json:"language_editions"`
	Relation       relationResponse       `json:"relation"`
	Copies         []copyResponse         `json:"copies"`
	// Placeholder marks a work a relation named but no scan has found: it is
	// reachable here and absent from the ordinary listing.
	Placeholder bool `json:"placeholder"`
}

func toWorkDetail(detail *works.WorkDetail) workDetailResponse {
	out := workDetailResponse{
		workSummaryResponse: toWorkSummary(detail.WorkSummary),
		SourceURL:           detail.SourceURL,
		CoverURL:            detail.CoverURL,
		SampleCoverURL:      detail.SampleCoverURL,
		SeriesID:            detail.SeriesID,
		SeriesName:          detail.SeriesName,
		VoiceActors:         toSourceEntityResponses(detail.VoiceActors),
		SourceTags:          toSourceEntityResponses(detail.SourceTags),
		Note:                detail.Note,
		Relation:            toRelationResponse(detail.Relation),
		Copies:              make([]copyResponse, 0, len(detail.Copies)),
		Placeholder:         detail.Placeholder,
	}
	for _, edition := range detail.Editions {
		out.Editions = append(out.Editions, editionResponse{
			VersionRootRJ: string(edition.VersionRootRJ),
			Language:      edition.Language,
			Label:         edition.Label,
			EditionID:     edition.EditionID,
			DisplayOrder:  edition.DisplayOrder,
		})
	}
	for _, copy := range detail.Copies {
		out.Copies = append(out.Copies, copyResponse{
			CopyID:    copy.ID,
			LibraryID: copy.LibraryID,
			RelPath:   copy.RelPath,
		})
	}
	return out
}

// getWork answers one work in full.
func (s *Server) getWork(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	detail, refusal := service.Detail(code)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	out := toWorkDetail(detail)
	s.attachLocalCovers(r, &out.workSummaryResponse)
	writeJSON(w, http.StatusOK, out)
}

// getWorkContent answers one work's file manifest (ADR 0013). The response
// is the works.Content itself: its types carry the manifest's JSON contract.
func (s *Server) getWorkContent(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	content, refusal := service.Content(code)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, content)
}

// ==================== metadata import ====================

// appliedGroupResponse is one group that was written.
type appliedGroupResponse struct {
	DataGroup string `json:"data_group"`
	DLCount   *int   `json:"dl_count,omitempty"`
}

// ignoredGroupResponse is one group that was valid but not written.
type ignoredGroupResponse struct {
	DataGroup string `json:"data_group"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
}

// warningResponse is one accepted disagreement the caller should know about.
type warningResponse struct {
	Code    string `json:"code"`
	WorkID  string `json:"work_id,omitempty"`
	Message string `json:"message"`
}

// importResponse separates what was written, what was valid but skipped, and
// what was written with a caveat. The three never overlap in meaning: a group
// in ignored was not written, a group in warnings was.
type importResponse struct {
	Applied             []appliedGroupResponse `json:"applied"`
	Ignored             []ignoredGroupResponse `json:"ignored"`
	Warnings            []warningResponse      `json:"warnings"`
	CreatedPlaceholders []string               `json:"created_placeholders"`
}

// putWorkMetadata imports one submission's data groups.
func (s *Server) putWorkMetadata(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	var req works.ImportRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid metadata payload")
		return
	}
	outcome, refusal := service.Import(code, req)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, toImportResponse(outcome))
}

func toImportResponse(outcome works.ImportOutcome) importResponse {
	out := importResponse{
		Applied:             make([]appliedGroupResponse, 0, len(outcome.Applied)),
		Ignored:             make([]ignoredGroupResponse, 0, len(outcome.Ignored)),
		Warnings:            make([]warningResponse, 0, len(outcome.Warnings)),
		CreatedPlaceholders: make([]string, 0, len(outcome.CreatedPlaceholders)),
	}
	for _, applied := range outcome.Applied {
		out.Applied = append(out.Applied, appliedGroupResponse{
			DataGroup: string(applied.DataGroup),
			DLCount:   applied.DLCount,
		})
	}
	for _, ignored := range outcome.Ignored {
		out.Ignored = append(out.Ignored, ignoredGroupResponse{
			DataGroup: string(ignored.DataGroup),
			Reason:    ignored.Reason,
			Message:   ignored.Message,
		})
	}
	for _, warning := range outcome.Warnings {
		out.Warnings = append(out.Warnings, warningResponse{
			Code:    warning.Code,
			WorkID:  string(warning.WorkID),
			Message: warning.Message,
		})
	}
	for _, placeholder := range outcome.CreatedPlaceholders {
		out.CreatedPlaceholders = append(out.CreatedPlaceholders, string(placeholder))
	}
	return out
}

// ==================== user data ====================

// userDataResponse is the personal data of one work.
type userDataResponse struct {
	Rating   *int   `json:"rating"`
	Favorite bool   `json:"favorite"`
	Note     string `json:"note"`
}

// patchWorkUserData applies a partial personal-data update. A field the request
// does not mention keeps its value; an explicit null rating clears it.
func (s *Server) patchWorkUserData(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	var req works.UserDataRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid user data payload")
		return
	}
	data, refusal := service.PatchUserData(code, req)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, userDataResponse{Rating: data.Rating, Favorite: data.Favorite, Note: data.Note})
}

// putWorkUserTags replaces the work's user tags with exactly the given set.
func (s *Server) putWorkUserTags(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	var req works.UserTagsRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid user tag payload")
		return
	}
	tags, refusal := service.ReplaceUserTags(code, req)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		UserTags []userTagResponse `json:"user_tags"`
	}{UserTags: toUserTagResponses(tags)})
}

// ==================== user tags ====================

// userTagRequest is the create/rename payload.
type userTagRequest struct {
	Name string `json:"name"`
}

// listUserTags answers every user tag.
func (s *Server) listUserTags(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	tags, refusal := service.UserTags()
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		UserTags []userTagResponse `json:"user_tags"`
	}{UserTags: toUserTagResponses(tags)})
}

// createUserTag creates a tag, or answers with the existing one when the name
// is already taken: creating by name is idempotent, so a caller never has to
// look a tag up before using it.
func (s *Server) createUserTag(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	var req userTagRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid user tag payload")
		return
	}
	tag, refusal := service.CreateUserTag(req.Name)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, userTagResponse{ID: tag.ID, Name: tag.Name})
}

// patchUserTag renames one tag.
func (s *Server) patchUserTag(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	var req userTagRequest
	if decodeErr := decodeJSON(w, r, &req); decodeErr != nil {
		writeDecodeError(w, decodeErr, "invalid user tag payload")
		return
	}
	tag, refusal := service.RenameUserTag(r.PathValue("id"), req.Name)
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	writeJSON(w, http.StatusOK, userTagResponse{ID: tag.ID, Name: tag.Name})
}

// deleteUserTag removes one tag and its relations, leaving every work alone.
func (s *Server) deleteUserTag(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	if refusal := service.DeleteUserTag(r.PathValue("id")); refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ==================== deletion ====================

// deleteWork permanently removes one work, at most.
func (s *Server) deleteWork(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	if refusal := service.Delete(code); refusal != nil {
		writeWorksError(w, refusal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
