package scraper

import (
	"encoding/json"

	"github.com/Clementine33/preen/backend/internal/works"
)

func asmrSnapshot(data record, at string) works.ImportRequest {
	// The shared edition report has the DLsite shape; provenance belongs to the fetched source.
	out := dlsiteSnapshot(
		record{"language_editions": data["language_editions"], "translation_info": data["translation_info"]},
		at,
	)
	if out.Editions != nil {
		out.Editions.Source = "asmr_one"
	}
	if out.Relations != nil {
		out.Relations.Source = "asmr_one"
	}
	if _, ok := data["title"]; !ok || !validASMRCore(data) {
		return out
	}
	core := &works.ImportCore{
		Source:      "asmr_one",
		FetchedAt:   at,
		Title:       scalar[string](data, "title"),
		NSFW:        scalar[bool](data, "nsfw"),
		ReleaseDate: scalar[string](data, "release"),
		SourceURL: scalar[string](
			data,
			"source_url",
		),
		CoverURL:       cleanURL(scalar[string](data, "mainCoverUrl")),
		SampleCoverURL: cleanURL(scalar[string](data, "samCoverUrl")),
		VoiceActors:    []works.ImportVoiceActor{},
		Tags:           []works.ImportTag{},
	}
	var circle struct {
		ID   json.RawMessage `json:"id"`
		Name string          `json:"name"`
	}
	if err := json.Unmarshal(data["circle"], &circle); err == nil && circle.Name != "" {
		core.Circle = &works.ImportCircle{SourceCircleID: entityID(circle.ID), Name: circle.Name}
	}
	var entities []struct {
		ID   json.RawMessage `json:"id"`
		Name string          `json:"name"`
	}
	if err := json.Unmarshal(data["vas"], &entities); err == nil {
		for _, actor := range entities {
			if actor.Name != "" {
				core.VoiceActors = append(
					core.VoiceActors,
					works.ImportVoiceActor{SourceVoiceActorID: entityID(actor.ID), Name: actor.Name},
				)
			}
		}
	}
	entities = nil
	if err := json.Unmarshal(data["tags"], &entities); err == nil {
		for _, tag := range entities {
			if tag.Name != "" {
				core.Tags = append(core.Tags, works.ImportTag{SourceTagID: entityID(tag.ID), Name: tag.Name})
			}
		}
	}
	out.Core = core
	return out
}

func validASMRCore(data record) bool {
	for _, key := range []string{"title", "release", "source_url", "mainCoverUrl", "samCoverUrl"} {
		if raw, ok := data[key]; ok {
			var text *string
			if json.Unmarshal(raw, &text) != nil {
				return false
			}
		}
	}
	if raw, ok := data["nsfw"]; ok {
		var nsfw *bool
		if json.Unmarshal(raw, &nsfw) != nil {
			return false
		}
	}
	var entity struct {
		ID   json.RawMessage `json:"id"`
		Name string          `json:"name"`
	}
	if raw, ok := data["circle"]; ok && json.Unmarshal(raw, &entity) != nil {
		return false
	}
	for _, key := range []string{"vas", "tags"} {
		if raw, ok := data[key]; ok && !validEntities(raw) {
			return false
		}
	}

	return true
}
