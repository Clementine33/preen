package scraper

import (
	"encoding/json"
	"strings"

	"github.com/Clementine33/preen/backend/internal/works"
)

type record map[string]json.RawMessage

func scalar[T any](r record, key string) *T {
	var value *T
	if err := json.Unmarshal(r[key], &value); err != nil {
		return nil
	}
	return value
}

func dlsiteSnapshot(data record, at string) works.ImportRequest {
	out := works.ImportRequest{Relations: relations(data, "dlsite", at)}
	out.Core = dlsiteCore(data, at)
	if raw, ok := data["language_editions"]; ok && string(raw) != "null" {
		var editions []struct {
			Workno       string          `json:"workno"`
			Lang         string          `json:"lang"`
			Label        *string         `json:"label"`
			EditionID    json.RawMessage `json:"edition_id"`
			DisplayOrder int             `json:"display_order"`
		}
		if err := json.Unmarshal(raw, &editions); err == nil {
			group := &works.ImportEditions{Source: "dlsite", FetchedAt: at, Items: []works.ImportEditionItem{}}
			for _, e := range editions {
				group.EditionID = entityID(e.EditionID)
				group.Items = append(
					group.Items,
					works.ImportEditionItem{
						VersionRootRJ: e.Workno,
						Language:      e.Lang,
						Label:         e.Label,
						DisplayOrder:  e.DisplayOrder,
					},
				)
			}
			out.Editions = group
		}
	}
	return out
}

func relations(data record, source, at string) *works.ImportRelations {
	var report struct {
		Original   bool     `json:"is_original"`
		Parent     bool     `json:"is_parent"`
		Child      bool     `json:"is_child"`
		OriginalRJ *string  `json:"original_workno"`
		ParentRJ   *string  `json:"parent_workno"`
		Children   []string `json:"child_worknos"`
		Language   *string  `json:"lang"`
	}
	if err := json.Unmarshal(data["translation_info"], &report); err != nil {
		return nil
	}
	role := ""
	roles := 0
	if report.Original {
		role = "original"
		roles++
	}
	if report.Parent {
		role = "parent"
		roles++
	}
	if report.Child {
		role = "child"
		roles++
	}
	if roles != 1 {
		return nil
	}
	return &works.ImportRelations{
		Source:     source,
		FetchedAt:  at,
		Role:       role,
		Language:   report.Language,
		OriginalRJ: report.OriginalRJ,
		ParentRJ:   report.ParentRJ,
		ChildRJs:   report.Children,
	}
}

func entityID(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if text == "" {
			return nil
		}
		return &text
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return nil
	}
	return new(number.String())
}

func cleanURL(value *string) *string {
	if value != nil && strings.HasPrefix(*value, "//") {
		return new("https:" + *value)
	}
	return value
}

// A malformed collection is not an empty collection: never clear stored credits on a decode failure.
func validDLsiteCore(data record) bool {
	for _, key := range []string{"options", "product_name", "maker_name", "regist_date"} {
		if raw, ok := data[key]; ok {
			var text *string
			if json.Unmarshal(raw, &text) != nil {
				return false
			}
		}
	}
	if raw, ok := data["age_category"]; ok {
		var age *int
		if json.Unmarshal(raw, &age) != nil {
			return false
		}
	}
	if raw, ok := data["genres"]; ok && !validEntities(raw) {
		return false
	}
	raw := data["creaters"]
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return true
	}
	var creators map[string]json.RawMessage
	if json.Unmarshal(raw, &creators) != nil {
		return false
	}
	if voices, ok := creators["voice_by"]; ok && !validEntities(voices) {
		return false
	}
	return true
}

func dlsiteCore(data record, at string) *works.ImportCore {
	if _, ok := data["product_name"]; !ok || !validDLsiteCore(data) {
		return nil
	}

	core := &works.ImportCore{Source: "dlsite", FetchedAt: at, Title: scalar[string](data, "product_name"),
		VoiceActors: []works.ImportVoiceActor{}, Tags: []works.ImportTag{}}
	if options := scalar[string](data, "options"); options != nil {
		core.Language = works.PreferredLanguage(strings.Split(*options, "#"))
	}
	if name := scalar[string](data, "maker_name"); name != nil && *name != "" {
		core.Circle = &works.ImportCircle{SourceCircleID: scalar[string](data, "maker_id"), Name: *name}
	}
	if age := scalar[int](data, "age_category"); age != nil {
		core.AgeCategory = age
		nsfw := *age == 3
		core.NSFW = &nsfw
	}
	if date := scalar[string](data, "regist_date"); date != nil && len(*date) >= 10 {
		value := (*date)[:10]
		core.ReleaseDate = &value
	}
	var creators map[string]json.RawMessage
	if json.Unmarshal(data["creaters"], &creators) == nil {
		var actors []sourceEntity
		if json.Unmarshal(creators["voice_by"], &actors) == nil {
			for _, actor := range actors {
				core.VoiceActors = append(
					core.VoiceActors,
					works.ImportVoiceActor{SourceVoiceActorID: entityID(actor.ID), Name: actor.Name},
				)
			}
		}
	}
	var genres []struct {
		ID   json.RawMessage `json:"id"`
		Name string          `json:"name"`
	}
	if err := json.Unmarshal(data["genres"], &genres); err == nil {
		for _, tag := range genres {
			if tag.Name != "" {
				core.Tags = append(core.Tags, works.ImportTag{SourceTagID: entityID(tag.ID), Name: tag.Name})
			}
		}
	}
	if raw := scalar[string](data, "workno"); raw != nil {
		core.SourceURL = new("https://www.dlsite.com/maniax/work/=/product_id/" + *raw + ".html")
	}
	var image struct {
		URL       string `json:"url"`
		ResizeURL string `json:"resize_url"`
	}
	if err := json.Unmarshal(data["image_main"], &image); err == nil && image.URL != "" {
		core.CoverURL = cleanURL(new(image.URL))
		if image.ResizeURL != "" {
			core.ThumbnailURL = new(
				"https://img.dlsite.jp/" + strings.TrimPrefix(image.ResizeURL, "/") + "_240x240.jpg",
			)
		}
	}
	image.URL = ""
	if err := json.Unmarshal(data["image_thum"], &image); err == nil && image.URL != "" {
		core.SampleCoverURL = cleanURL(new(image.URL))
	}
	return core
}

type sourceEntity struct {
	ID   json.RawMessage `json:"id"`
	Name string          `json:"name"`
}

func validEntities(raw json.RawMessage) bool {
	var list []sourceEntity
	if json.Unmarshal(raw, &list) != nil {
		return false
	}
	for _, entity := range list {
		if strings.TrimSpace(entity.Name) == "" {
			return false
		}
		if len(entity.ID) > 0 && string(entity.ID) != "null" && string(entity.ID) != `""` &&
			entityID(entity.ID) == nil {
			return false
		}
	}
	return true
}
