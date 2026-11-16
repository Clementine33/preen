package httpapi

import (
	"encoding/json"

	"github.com/Clementine33/preen/backend/internal/works"
)

// relationWarningResponse is one accepted relation disagreement on a work detail.
type relationWarningResponse struct {
	Code    string `json:"code"`
	WorkID  string `json:"rj"`
	Message string `json:"message"`
}

// workContentResponse is the GET /api/v1/works/{rj}/content payload.
type workContentResponse struct {
	RJ     string                    `json:"rj"`
	Copies []workContentCopyResponse `json:"copies"`
}

// workContentCopyResponse is one bound copy's manifest entry.
type workContentCopyResponse struct {
	CopyID    string                   `json:"copy_id"`
	LibraryID string                   `json:"library_id"`
	DirID     string                   `json:"dir_id"`
	Online    bool                     `json:"online"`
	Tree      *workContentNodeResponse `json:"tree,omitempty"`
}

// workContentNodeResponse mirrors works.ContentNode's two-shape JSON union
// (directory vs file).
type workContentNodeResponse struct {
	Name        string
	RelPath     string
	Children    []workContentNodeResponse
	Kind        string
	Size        int64
	Format      *string
	Bitrate     *int32
	Width       *int
	Height      *int
	DurationSec *float64
	ThumbV      string
}

type workContentDirNodeJSON struct {
	Name     string                    `json:"name"`
	RelPath  string                    `json:"rel_path"`
	Children []workContentNodeResponse `json:"children"`
}

type workContentFileNodeJSON struct {
	Name        string   `json:"name"`
	RelPath     string   `json:"rel_path"`
	Kind        string   `json:"kind"`
	Size        int64    `json:"size"`
	Format      *string  `json:"format"`
	Bitrate     *int32   `json:"bitrate"`
	Width       *int     `json:"width"`
	Height      *int     `json:"height"`
	DurationSec *float64 `json:"duration_sec"`
	ThumbV      *string  `json:"thumb_v"`
}

// MarshalJSON renders the directory and file node shapes identically to works.ContentNode.
func (n workContentNodeResponse) MarshalJSON() ([]byte, error) {
	if n.Kind == "" {
		children := n.Children
		if children == nil {
			children = []workContentNodeResponse{}
		}
		return json.Marshal(workContentDirNodeJSON{Name: n.Name, RelPath: n.RelPath, Children: children})
	}
	var thumbV *string
	if n.ThumbV != "" {
		thumbV = &n.ThumbV
	}
	return json.Marshal(workContentFileNodeJSON{
		Name: n.Name, RelPath: n.RelPath, Kind: n.Kind, Size: n.Size,
		Format: n.Format, Bitrate: n.Bitrate, Width: n.Width, Height: n.Height,
		DurationSec: n.DurationSec, ThumbV: thumbV,
	})
}

func toWorkContentNodeResponse(n *works.ContentNode) *workContentNodeResponse {
	if n == nil {
		return nil
	}
	var children []workContentNodeResponse
	if n.Children != nil {
		children = make([]workContentNodeResponse, len(n.Children))
		for i := range n.Children {
			children[i] = *toWorkContentNodeResponse(&n.Children[i])
		}
	}
	return &workContentNodeResponse{
		Name:        n.Name,
		RelPath:     n.RelPath,
		Children:    children,
		Kind:        string(n.Kind),
		Size:        n.Size,
		Format:      n.Format,
		Bitrate:     n.Bitrate,
		Width:       n.Width,
		Height:      n.Height,
		DurationSec: n.DurationSec,
		ThumbV:      n.ThumbV,
	}
}

func toWorkContentResponse(c *works.Content) workContentResponse {
	copies := make([]workContentCopyResponse, len(c.Copies))
	for i, cp := range c.Copies {
		copies[i] = workContentCopyResponse{
			CopyID:    cp.CopyID,
			LibraryID: cp.LibraryID,
			DirID:     cp.DirID,
			Online:    cp.Online,
			Tree:      toWorkContentNodeResponse(cp.Tree),
		}
	}
	return workContentResponse{RJ: c.RJ, Copies: copies}
}

// workMetadataImportRequest is the PUT /api/v1/works/{rj}/metadata payload.
type workMetadataImportRequest struct {
	Core       *workImportCoreRequest       `json:"core"`
	Statistics *workImportStatisticsRequest `json:"statistics"`
	Relations  *workImportRelationsRequest  `json:"relations"`
	Editions   *workImportEditionsRequest   `json:"editions"`
}

type workImportCoreRequest struct {
	Source         string                        `json:"source"`
	FetchedAt      string                        `json:"fetched_at"`
	Title          *string                       `json:"title"`
	Language       *string                       `json:"language"`
	Circle         *workImportCircleRequest      `json:"circle"`
	NSFW           *bool                         `json:"nsfw"`
	AgeCategory    *int                          `json:"age_category"`
	ReleaseDate    *string                       `json:"release_date"`
	SourceURL      *string                       `json:"source_url"`
	CoverURL       *string                       `json:"cover_url"`
	SampleCoverURL *string                       `json:"sample_cover_url"`
	SeriesID       *string                       `json:"series_id"`
	SeriesName     *string                       `json:"series_name"`
	VoiceActors    []workImportVoiceActorRequest `json:"voice_actors"`
	Tags           []workImportTagRequest        `json:"tags"`
}

type workImportCircleRequest struct {
	SourceCircleID *string `json:"source_circle_id"`
	Name           string  `json:"name"`
}

type workImportVoiceActorRequest struct {
	SourceVoiceActorID *string `json:"source_voice_actor_id"`
	Name               string  `json:"name"`
}

type workImportTagRequest struct {
	SourceTagID *string `json:"source_tag_id"`
	Name        string  `json:"name"`
}

type workImportStatisticsRequest struct {
	FetchedAt string `json:"fetched_at"`
	DLCount   *int   `json:"dl_count"`
}

type workImportRelationsRequest struct {
	Source     string   `json:"source"`
	FetchedAt  string   `json:"fetched_at"`
	Role       string   `json:"role"`
	Language   *string  `json:"language"`
	OriginalRJ *string  `json:"original_rj"`
	ParentRJ   *string  `json:"parent_rj"`
	ChildRJs   []string `json:"child_rjs"`
}

type workImportEditionsRequest struct {
	Source    string                         `json:"source"`
	FetchedAt string                         `json:"fetched_at"`
	EditionID *string                        `json:"edition_id"`
	Items     []workImportEditionItemRequest `json:"items"`
}

type workImportEditionItemRequest struct {
	VersionRootRJ string  `json:"version_root_rj"`
	Language      string  `json:"language"`
	Label         *string `json:"label"`
	DisplayOrder  int     `json:"display_order"`
}

func (r workMetadataImportRequest) toWorks() works.ImportRequest {
	out := works.ImportRequest{}
	if r.Core != nil {
		out.Core = toWorksImportCore(r.Core)
	}
	if r.Statistics != nil {
		out.Statistics = &works.ImportStatistics{
			FetchedAt: r.Statistics.FetchedAt,
			DLCount:   r.Statistics.DLCount,
		}
	}
	if r.Relations != nil {
		out.Relations = &works.ImportRelations{
			Source:     r.Relations.Source,
			FetchedAt:  r.Relations.FetchedAt,
			Role:       r.Relations.Role,
			Language:   r.Relations.Language,
			OriginalRJ: r.Relations.OriginalRJ,
			ParentRJ:   r.Relations.ParentRJ,
			ChildRJs:   r.Relations.ChildRJs,
		}
	}
	if r.Editions != nil {
		out.Editions = toWorksImportEditions(r.Editions)
	}
	return out
}

func toWorksImportCore(c *workImportCoreRequest) *works.ImportCore {
	out := &works.ImportCore{
		Source:         c.Source,
		FetchedAt:      c.FetchedAt,
		Title:          c.Title,
		Language:       c.Language,
		NSFW:           c.NSFW,
		AgeCategory:    c.AgeCategory,
		ReleaseDate:    c.ReleaseDate,
		SourceURL:      c.SourceURL,
		CoverURL:       c.CoverURL,
		SampleCoverURL: c.SampleCoverURL,
		SeriesID:       c.SeriesID,
		SeriesName:     c.SeriesName,
	}
	if c.Circle != nil {
		out.Circle = &works.ImportCircle{SourceCircleID: c.Circle.SourceCircleID, Name: c.Circle.Name}
	}
	if c.VoiceActors != nil {
		out.VoiceActors = make([]works.ImportVoiceActor, len(c.VoiceActors))
		for i, va := range c.VoiceActors {
			out.VoiceActors[i] = works.ImportVoiceActor{
				SourceVoiceActorID: va.SourceVoiceActorID,
				Name:               va.Name,
			}
		}
	}
	if c.Tags != nil {
		out.Tags = make([]works.ImportTag, len(c.Tags))
		for i, tag := range c.Tags {
			out.Tags[i] = works.ImportTag{SourceTagID: tag.SourceTagID, Name: tag.Name}
		}
	}
	return out
}

func toWorksImportEditions(e *workImportEditionsRequest) *works.ImportEditions {
	out := &works.ImportEditions{
		Source:    e.Source,
		FetchedAt: e.FetchedAt,
		EditionID: e.EditionID,
	}
	if e.Items != nil {
		out.Items = make([]works.ImportEditionItem, len(e.Items))
		for i, item := range e.Items {
			out.Items[i] = works.ImportEditionItem{
				VersionRootRJ: item.VersionRootRJ,
				Language:      item.Language,
				Label:         item.Label,
				DisplayOrder:  item.DisplayOrder,
			}
		}
	}
	return out
}

// workUserDataPatchRequest is the PATCH /api/v1/works/{rj}/user-data payload.
type workUserDataPatchRequest struct {
	Rating   nullableInt `json:"rating"`
	Favorite *bool       `json:"favorite"`
	Note     *string     `json:"note"`
}

func (r workUserDataPatchRequest) toWorks() works.UserDataRequest {
	return works.UserDataRequest{
		Rating:   r.Rating.toWorks(),
		Favorite: r.Favorite,
		Note:     r.Note,
	}
}

// workUserTagsRequest is the PUT /api/v1/works/{rj}/user-tags payload.
type workUserTagsRequest struct {
	TagIDs []string `json:"tag_ids"`
}

// userTagListResponse is the response for user tag list/replace endpoints.
type userTagListResponse struct {
	UserTags []userTagResponse `json:"user_tags"`
}
