package works

import "time"

// ImportRequest is the PUT /works/{rj}/metadata body: up to four independent
// data groups. An absent group is left exactly as it was; a present group is a
// complete snapshot of that group, so an unknown scalar is an explicit null and
// an empty array means "the set is empty".
//
// Timestamps arrive as strings and are parsed here rather than by the JSON
// decoder: the decoder would accept the whole RFC 3339 family, and the catalog
// stores exactly one form.
type ImportRequest struct {
	Core       *ImportCore       `json:"core"`
	Statistics *ImportStatistics `json:"statistics"`
	Relations  *ImportRelations  `json:"relations"`
	Editions   *ImportEditions   `json:"editions"`
}

// ImportCore is the basic description: what the work is called, who made it,
// its declared language, when it was released and how it looks.
type ImportCore struct {
	Source         string             `json:"source"`
	FetchedAt      string             `json:"fetched_at"`
	Title          *string            `json:"title"`
	Language       *string            `json:"language"`
	Circle         *ImportCircle      `json:"circle"`
	NSFW           *bool              `json:"nsfw"`
	AgeCategory    *int               `json:"age_category"`
	ReleaseDate    *string            `json:"release_date"`
	SourceURL      *string            `json:"source_url"`
	CoverURL       *string            `json:"cover_url"`
	ThumbnailURL   *string            `json:"thumbnail_url"`
	SampleCoverURL *string            `json:"sample_cover_url"`
	SeriesID       *string            `json:"series_id"`
	SeriesName     *string            `json:"series_name"`
	VoiceActors    []ImportVoiceActor `json:"voice_actors"`
	Tags           []ImportTag        `json:"tags"`
}

// ImportCircle is the circle that made the work.
type ImportCircle struct {
	SourceCircleID *string `json:"source_circle_id"`
	Name           string  `json:"name"`
}

// ImportVoiceActor is one credited performer.
type ImportVoiceActor struct {
	SourceVoiceActorID *string `json:"source_voice_actor_id"`
	Name               string  `json:"name"`
}

// ImportTag is one source tag.
type ImportTag struct {
	SourceTagID *string `json:"source_tag_id"`
	Name        string  `json:"name"`
}

// ImportStatistics is the sales snapshot. It carries no source: sales are only
// ever reported by DLsite, so there is nothing to choose between.
type ImportStatistics struct {
	FetchedAt string `json:"fetched_at"`
	DLCount   *int   `json:"dl_count"`
}

// ImportRelations is one work's own relation report: its role, the language of
// this edition, and the works it points at. It is a snapshot of this work's
// relations only - the declared children are the child_rjs it names.
type ImportRelations struct {
	Source     string   `json:"source"`
	FetchedAt  string   `json:"fetched_at"`
	Role       string   `json:"role"`
	Language   *string  `json:"language"`
	OriginalRJ *string  `json:"original_rj"`
	ParentRJ   *string  `json:"parent_rj"`
	ChildRJs   []string `json:"child_rjs"`
}

// ImportEditions is the language edition list a work declares.
type ImportEditions struct {
	Source    string              `json:"source"`
	FetchedAt string              `json:"fetched_at"`
	EditionID *string             `json:"edition_id"`
	Items     []ImportEditionItem `json:"items"`
}

// ImportEditionItem is one declared language edition.
type ImportEditionItem struct {
	VersionRootRJ string  `json:"version_root_rj"`
	Language      string  `json:"language"`
	Label         *string `json:"label"`
	DisplayOrder  int     `json:"display_order"`
}

// EntityInput is one submitted circle, voice actor or tag, normalized to the
// shape the storage layer writes.
type EntityInput struct {
	SourceID *string
	Name     string
}

// CoreGroup is a validated basic description. The four group payloads below
// are what a submission becomes once its contract has been checked.
type CoreGroup struct {
	Source         Source
	FetchedAt      time.Time
	Title          *string
	Language       *string
	Circle         *EntityInput
	NSFW           *bool
	AgeCategory    *int
	ReleaseDate    *string
	SourceURL      *string
	CoverURL       *string
	ThumbnailURL   *string
	SampleCoverURL *string
	SeriesID       *string
	SeriesName     *string
	VoiceActors    []EntityInput
	Tags           []EntityInput
}

// StatisticsGroup is a validated sales snapshot.
type StatisticsGroup struct {
	FetchedAt time.Time
	DLCount   *int
}

// RelationsGroup is a validated relation report.
type RelationsGroup struct {
	Source     Source
	FetchedAt  time.Time
	Role       RelationRole
	Language   *string
	OriginalRJ *WorkCode
	ParentRJ   *WorkCode
	ChildRJs   []WorkCode
}

// EditionsGroup is a validated language edition list.
type EditionsGroup struct {
	Source    Source
	FetchedAt time.Time
	EditionID *string
	Items     []LanguageEdition
}

// ValidatedImport is a submission that passed the contract: its numbers are
// canonical, its enums are known and its timestamps are in the stored form.
// What remains to be decided - whether a group supersedes what is stored, and
// whether the relation is structurally legal against the rest of the catalog -
// belongs to the storage transaction.
type ValidatedImport struct {
	Target     WorkCode
	Core       *CoreGroup
	Statistics *StatisticsGroup
	Relations  *RelationsGroup
	Editions   *EditionsGroup
}

// Groups lists the data groups this submission carries, in a fixed order.
func (v ValidatedImport) Groups() []DataGroup {
	groups := make([]DataGroup, 0, 4)
	if v.Core != nil {
		groups = append(groups, GroupCore)
	}
	if v.Statistics != nil {
		groups = append(groups, GroupStatistics)
	}
	if v.Relations != nil {
		groups = append(groups, GroupRelations)
	}
	if v.Editions != nil {
		groups = append(groups, GroupEditions)
	}
	return groups
}

// ValidateImport checks one submission against the contract. It refuses the
// whole submission - there is no per-group rejection mode, so an illegal
// number in one group never leaves another group half written.
func ValidateImport(target WorkCode, req ImportRequest) (*ValidatedImport, *Error) {
	out := &ValidatedImport{Target: target}
	if req.Core == nil && req.Statistics == nil && req.Relations == nil && req.Editions == nil {
		return nil, Invalid(CodeInvalidArgument, "at least one data group is required")
	}

	if req.Core != nil {
		core, err := validateCore(*req.Core)
		if err != nil {
			return nil, err
		}
		out.Core = core
	}
	if req.Statistics != nil {
		statistics, err := validateStatistics(*req.Statistics)
		if err != nil {
			return nil, err
		}
		out.Statistics = statistics
	}
	if req.Relations != nil {
		relations, err := validateRelationsPayload(target, *req.Relations)
		if err != nil {
			return nil, err
		}
		out.Relations = relations
	}
	if req.Editions != nil {
		editions, err := validateEditions(*req.Editions)
		if err != nil {
			return nil, err
		}
		out.Editions = editions
	}
	return out, nil
}

func validateSource(raw string) (Source, *Error) {
	source := Source(raw)
	if !source.Valid() {
		return "", Invalid(CodeInvalidArgument, "source must be dlsite or asmr_one")
	}
	return source, nil
}

func validateFetchedAt(raw string) (time.Time, *Error) {
	at, err := parseTimestamp(raw)
	if err != nil {
		return time.Time{}, NewError(
			KindInvalid,
			CodeTimestampFormat,
			"fetched_at must be UTC with nine fractional digits, e.g. 2026-09-27T10:00:00.000000000Z",
		)
	}
	return at, nil
}

func validateCore(core ImportCore) (*CoreGroup, *Error) {
	source, err := validateSource(core.Source)
	if err != nil {
		return nil, err
	}
	fetchedAt, err := validateFetchedAt(core.FetchedAt)
	if err != nil {
		return nil, err
	}
	if core.Language != nil && PreferredLanguage([]string{*core.Language}) == nil {
		return nil, Invalid(CodeInvalidArgument, "language must be a supported language code")
	}
	if !validAgeCategory(core.AgeCategory) {
		return nil, Invalid(CodeInvalidArgument, "age_category must be 1, 2 or 3")
	}
	if core.AgeCategory != nil {
		nsfw := *core.AgeCategory == 3
		if core.NSFW != nil && *core.NSFW != nsfw {
			return nil, Invalid(CodeInvalidArgument, "nsfw conflicts with age_category")
		}
		core.NSFW = &nsfw
	} else if core.NSFW != nil && *core.NSFW {
		core.AgeCategory = new(3)
	}
	if core.ReleaseDate != nil && !validDate(*core.ReleaseDate) {
		return nil, Invalid(CodeInvalidArgument, "release_date must be YYYY-MM-DD")
	}

	out := &CoreGroup{
		Source:         source,
		FetchedAt:      fetchedAt,
		Title:          core.Title,
		Language:       core.Language,
		NSFW:           core.NSFW,
		AgeCategory:    core.AgeCategory,
		ReleaseDate:    core.ReleaseDate,
		SourceURL:      core.SourceURL,
		CoverURL:       core.CoverURL,
		ThumbnailURL:   core.ThumbnailURL,
		SampleCoverURL: core.SampleCoverURL,
		SeriesID:       core.SeriesID,
		SeriesName:     core.SeriesName,
	}
	if core.Circle != nil {
		if core.Circle.Name == "" {
			return nil, Invalid(CodeInvalidArgument, "circle.name is required")
		}
		out.Circle = &EntityInput{SourceID: core.Circle.SourceCircleID, Name: core.Circle.Name}
	}
	for _, actor := range core.VoiceActors {
		if actor.Name == "" {
			return nil, Invalid(CodeInvalidArgument, "every voice actor needs a name")
		}
		out.VoiceActors = append(out.VoiceActors, EntityInput{SourceID: actor.SourceVoiceActorID, Name: actor.Name})
	}
	for _, tag := range core.Tags {
		if tag.Name == "" {
			return nil, Invalid(CodeInvalidArgument, "every tag needs a name")
		}
		out.Tags = append(out.Tags, EntityInput{SourceID: tag.SourceTagID, Name: tag.Name})
	}
	return out, nil
}

func validateStatistics(statistics ImportStatistics) (*StatisticsGroup, *Error) {
	fetchedAt, err := validateFetchedAt(statistics.FetchedAt)
	if err != nil {
		return nil, err
	}
	if statistics.DLCount != nil && *statistics.DLCount < 0 {
		return nil, Invalid(CodeInvalidArgument, "dl_count must not be negative")
	}
	return &StatisticsGroup{FetchedAt: fetchedAt, DLCount: statistics.DLCount}, nil
}

func validateRelationsPayload(target WorkCode, payload ImportRelations) (*RelationsGroup, *Error) {
	source, err := validateSource(payload.Source)
	if err != nil {
		return nil, err
	}
	fetchedAt, err := validateFetchedAt(payload.FetchedAt)
	if err != nil {
		return nil, err
	}
	role := RelationRole(payload.Role)
	switch role {
	case RoleOriginal, RoleParent, RoleChild:
	default:
		return nil, Invalid(CodeInvalidArgument, "role must be original, parent or child")
	}

	out := &RelationsGroup{Source: source, FetchedAt: fetchedAt, Role: role, Language: payload.Language}
	if payload.OriginalRJ != nil {
		original, ok := ParseWorkCode(*payload.OriginalRJ)
		if !ok {
			return nil, Invalid(CodeInvalidArgument, "original_rj is not a work number")
		}
		if original == target {
			return nil, Invalid(CodeRelationSelfReference, "a work cannot be its own original")
		}
		out.OriginalRJ = &original
	}
	if payload.ParentRJ != nil {
		parent, ok := ParseWorkCode(*payload.ParentRJ)
		if !ok {
			return nil, Invalid(CodeInvalidArgument, "parent_rj is not a work number")
		}
		if parent == target {
			return nil, Invalid(CodeRelationSelfReference, "a work cannot be its own parent")
		}
		out.ParentRJ = &parent
	}
	seen := map[WorkCode]struct{}{}
	for _, raw := range payload.ChildRJs {
		child, ok := ParseWorkCode(raw)
		if !ok {
			return nil, Invalid(CodeInvalidArgument, "child_rjs contains something that is not a work number")
		}
		if child == target {
			return nil, Invalid(CodeRelationSelfReference, "a work cannot declare itself a child")
		}
		if _, dup := seen[child]; dup {
			continue
		}
		seen[child] = struct{}{}
		out.ChildRJs = append(out.ChildRJs, child)
	}
	return out, nil
}

func validateEditions(payload ImportEditions) (*EditionsGroup, *Error) {
	source, err := validateSource(payload.Source)
	if err != nil {
		return nil, err
	}
	fetchedAt, err := validateFetchedAt(payload.FetchedAt)
	if err != nil {
		return nil, err
	}
	out := &EditionsGroup{Source: source, FetchedAt: fetchedAt, EditionID: payload.EditionID}
	seen := map[string]struct{}{}
	for _, item := range payload.Items {
		root, ok := ParseWorkCode(item.VersionRootRJ)
		if !ok {
			return nil, Invalid(CodeInvalidArgument, "version_root_rj is not a work number")
		}
		if item.Language == "" {
			return nil, Invalid(CodeInvalidArgument, "every edition needs a language")
		}
		if item.DisplayOrder < 0 {
			return nil, Invalid(CodeInvalidArgument, "display_order must not be negative")
		}
		// One language appears once per work: the list is the set of language
		// editions, and the table's key says the same thing.
		if _, dup := seen[item.Language]; dup {
			return nil, Invalid(CodeInvalidArgument, "an edition language appears twice")
		}
		seen[item.Language] = struct{}{}
		out.Items = append(out.Items, LanguageEdition{
			VersionRootRJ: root,
			Language:      item.Language,
			Label:         item.Label,
			EditionID:     payload.EditionID,
			DisplayOrder:  item.DisplayOrder,
		})
	}
	return out, nil
}

// AppliedGroup records a group that was written.
type AppliedGroup struct {
	DataGroup DataGroup `json:"data_group"`
	DLCount   *int      `json:"dl_count,omitempty"`
}

// IgnoredGroup records a group that was valid but not written, with the reason.
type IgnoredGroup struct {
	DataGroup DataGroup `json:"data_group"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
}

// ImportOutcome is the result of one submission: what was written, what was
// valid but skipped, what was accepted with a caveat, and which works were
// created as relation placeholders along the way.
type ImportOutcome struct {
	Applied             []AppliedGroup    `json:"applied"`
	Ignored             []IgnoredGroup    `json:"ignored"`
	Warnings            []RelationWarning `json:"warnings"`
	CreatedPlaceholders []WorkCode        `json:"created_placeholders"`
}

func validAgeCategory(age *int) bool {
	return age == nil || *age >= 1 && *age <= 3
}
