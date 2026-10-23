package works

import "strings"

// Source is where a piece of metadata came from.
type Source string

// The accepted sources. DLsite outranks ASMR.one wherever both describe the
// same data group.
const (
	SourceDLsite  Source = "dlsite"
	SourceASMRone Source = "asmr_one"
)

// Valid reports whether the source is one this build accepts.
func (s Source) Valid() bool { return s == SourceDLsite || s == SourceASMRone }

// Outranks reports whether this source's description of a data group replaces
// the other's. DLsite does; the reverse never does.
func (s Source) Outranks(other Source) bool { return s == SourceDLsite && other == SourceASMRone }

// DataGroup names one independently submittable slice of source metadata.
// The split is not cosmetic: sales come from DLsite's dynamic endpoint, while
// the language editions only exist on its static one, so a crawler refreshing
// sales cheaply must not be forced to resubmit - or worse, silently blank -
// the groups it did not fetch.
type DataGroup string

// The four groups a submission may carry.
const (
	GroupCore       DataGroup = "core"
	GroupStatistics DataGroup = "statistics"
	GroupRelations  DataGroup = "relations"
	GroupEditions   DataGroup = "editions"
)

// StatisticsSource is the only source sales may come from.
const StatisticsSource = SourceDLsite

// Ignore reasons a submitted group can be valid but not written.
const (
	ReasonStaleFetchedAt      = "STALE_FETCHED_AT"
	ReasonLowerPrioritySource = "LOWER_PRIORITY_SOURCE"
)

// SourceState is what the catalog already holds for one data group.
type SourceState struct {
	Source    Source
	FetchedAt int64 // Unix nanoseconds; the stored form is fixed-width UTC text
}

// GroupDecision is whether a submitted group is written, and why not.
type GroupDecision struct {
	Apply   bool
	Reason  string
	Message string
}

// DecideGroup applies the per-group source rules. Priority decides first: a
// higher-priority source replaces a lower-priority one outright, so an older
// DLsite snapshot still displaces a newer ASMR.one one. The fetch-time rule is
// scoped to the same source, where it is the only thing that can order two
// descriptions of the same thing. A group with nothing stored is always
// applied.
//
// Statistics carry no source of their own - sales only exist on DLsite - so
// for that group the fetch time is the only question there is.
func DecideGroup(group DataGroup, incoming Source, fetchedAt int64, existing SourceState, present bool) GroupDecision {
	if !present {
		return GroupDecision{Apply: true}
	}
	if group == GroupStatistics {
		if fetchedAt <= existing.FetchedAt {
			return staleDecision()
		}
		return GroupDecision{Apply: true}
	}
	switch {
	case incoming.Outranks(existing.Source):
		return GroupDecision{Apply: true}
	case existing.Source.Outranks(incoming):
		return GroupDecision{
			Apply:   false,
			Reason:  ReasonLowerPrioritySource,
			Message: string(existing.Source) + " data is already stored and outranks " + string(incoming),
		}
	case fetchedAt <= existing.FetchedAt:
		return staleDecision()
	default:
		return GroupDecision{Apply: true}
	}
}

// staleDecision is the shared "the stored fetch is at least as new" outcome.
func staleDecision() GroupDecision {
	return GroupDecision{
		Apply:   false,
		Reason:  ReasonStaleFetchedAt,
		Message: "the stored fetch is at least as new as this one",
	}
}

// SourceEntity is one circle, voice actor or tag as it is stored. SourceID is
// the identifier the source assigned; when it is absent the entity can only be
// recognized within one work, which is what WorkID records.
type SourceEntity struct {
	ID       string
	Source   Source
	SourceID *string
	Name     string
	WorkID   *WorkCode
}

// UserTag is one user-defined tag.
type UserTag struct {
	ID   string
	Name string
}

// NormalizeUserTagName trims the name and reports whether it is usable. Names
// are compared exactly after trimming: case is not folded, and a source tag
// with a similar name is a different thing.
func NormalizeUserTagName(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	return trimmed, trimmed != ""
}

// Copy is one directory bound to a work.
type Copy struct {
	ID        string
	LibraryID string
	RelPath   string
}

// LanguageEdition is one language edition a work declared.
type LanguageEdition struct {
	VersionRootRJ WorkCode
	Language      string
	Label         *string
	EditionID     *string
	DisplayOrder  int
}

// WorkSummary is one row of the ordinary listing. It is deliberately not the
// whole work: the list carries what a grid shows, and the detail carries the
// rest.
type WorkSummary struct {
	ID           WorkCode
	Title        *string
	ThumbnailURL *string
	CircleName   *string
	Language     *string
	NSFW         *bool
	AgeCategory  *int
	ReleaseDate  *string
	DLCount      *int
	Rating       *int
	Favorite     bool
	UserTags     []UserTag
	HasNote      bool
	CollectedAt  string
	CopyCount    int
	Listening    ListeningSummary
}

// WorkDetail is everything the detail endpoint answers with.
type WorkDetail struct {
	WorkSummary

	SourceURL      *string
	CoverURL       *string
	SampleCoverURL *string
	SeriesID       *string
	SeriesName     *string
	VoiceActors    []SourceEntity
	SourceTags     []SourceEntity
	Note           string
	Editions       []LanguageEdition
	Relation       ResolvedRelation
	// EditionTree is the resolved edition lines (BuildEditionTree); nil when
	// the work has no edition besides itself.
	EditionTree []EditionSlot
	Copies      []Copy
	// CollectionIDs are the user collections holding the work, in list order;
	// Favorites is stated by Favorite instead.
	CollectionIDs []string
	// Listening holds one row per Rendition that has been played.
	Listening []Listening
}
