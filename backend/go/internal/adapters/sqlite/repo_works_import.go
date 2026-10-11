package sqlite

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/onsei/organizer/backend/internal/works"
)

// ImportMetadata applies one validated submission inside a single transaction:
// the groups that supersede what is stored are written, the works they name are
// created as placeholders if they do not exist yet, and every derived relation
// the change affects is recomputed. Any refusal rolls the whole thing back, so
// a submission never leaves some groups written and others not.
func (r *Repository) ImportMetadata(in *works.ValidatedImport, now time.Time) (works.ImportOutcome, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return works.ImportOutcome{}, err
	}
	defer tx.Rollback()

	// A submission describes a work the catalog already knows. A work becomes
	// known by being found on disk; an import is not a second way to create one,
	// which is what keeps the directory name the single authority on identity.
	if exists, existsErr := workExists(tx, in.Target); existsErr != nil {
		return works.ImportOutcome{}, existsErr
	} else if !exists {
		return works.ImportOutcome{}, works.NotFound(works.CodeWorkNotFound, "no such work")
	}

	outcome := works.ImportOutcome{}
	storedStates, err := loadSourceStates(tx, in.Target)
	if err != nil {
		return works.ImportOutcome{}, err
	}

	// Groups are applied in a fixed order, and relations last among those that
	// create placeholders: the relation rules need every work they mention to
	// have a row, and they run once, over the catalog as the submission leaves
	// it.
	if in.Core != nil {
		if err := applyCoreGroup(tx, in.Target, in.Core, storedStates, &outcome, now); err != nil {
			return works.ImportOutcome{}, err
		}
	}
	if in.Statistics != nil {
		if err := applyStatisticsGroup(tx, in.Target, in.Statistics, storedStates, &outcome); err != nil {
			return works.ImportOutcome{}, err
		}
	}
	if in.Editions != nil {
		if err := applyEditionsGroup(tx, in.Target, in.Editions, storedStates, &outcome, now); err != nil {
			return works.ImportOutcome{}, err
		}
	}
	if in.Relations != nil {
		warnings, err := applyRelationsGroup(tx, in.Target, in.Relations, storedStates, &outcome, now)
		if err != nil {
			return works.ImportOutcome{}, err
		}
		outcome.Warnings = warnings
	}

	if err := tx.Commit(); err != nil {
		return works.ImportOutcome{}, err
	}
	r.catalogChanged()
	return outcome, nil
}

// loadSourceStates reads what the catalog already holds per data group.
func loadSourceStates(tx *sql.Tx, id works.WorkCode) (map[works.DataGroup]works.SourceState, error) {
	rows, err := tx.Query(
		"SELECT data_group, source, fetched_at FROM work_source_state WHERE work_id = ?", string(id),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := map[works.DataGroup]works.SourceState{}
	for rows.Next() {
		var (
			group   string
			source  string
			fetched string
		)
		if err := rows.Scan(&group, &source, &fetched); err != nil {
			return nil, err
		}
		at, err := time.Parse(works.TimestampFormat, fetched)
		if err != nil {
			return nil, err
		}
		states[works.DataGroup(group)] = works.SourceState{Source: works.Source(source), FetchedAt: at.UnixNano()}
	}
	return states, rows.Err()
}

// recordGroup marks a group as applied, or as ignored with its reason.
func recordGroup(
	outcome *works.ImportOutcome,
	group works.DataGroup,
	decision works.GroupDecision,
	applied works.AppliedGroup,
) bool {
	if !decision.Apply {
		outcome.Ignored = append(outcome.Ignored, works.IgnoredGroup{
			DataGroup: group,
			Reason:    decision.Reason,
			Message:   decision.Message,
		})
		return false
	}
	outcome.Applied = append(outcome.Applied, applied)
	return true
}

// touchSourceState records the fetch that a group's data now comes from.
func touchSourceState(tx *sql.Tx, id works.WorkCode, group works.DataGroup, source works.Source, at time.Time) error {
	_, err := tx.Exec(`
		INSERT INTO work_source_state (work_id, data_group, source, fetched_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(work_id, data_group) DO UPDATE SET source = excluded.source, fetched_at = excluded.fetched_at
	`, string(id), string(group), string(source), formatWorksTimestamp(at))
	return err
}

// applyCoreGroup replaces the work's basic description. The group is a complete
// snapshot, so a field the submission left out is cleared rather than kept: an
// absent title means "the source reports no title", not "keep the old one".
func applyCoreGroup(
	tx *sql.Tx,
	target works.WorkCode,
	group *works.CoreGroup,
	stored map[works.DataGroup]works.SourceState,
	outcome *works.ImportOutcome,
	now time.Time,
) error {
	state, present := stored[works.GroupCore]
	decision := works.DecideGroup(works.GroupCore, group.Source, group.FetchedAt.UnixNano(), state, present)
	if !recordGroup(outcome, works.GroupCore, decision, works.AppliedGroup{DataGroup: works.GroupCore}) {
		return nil
	}

	var circleID *string
	if group.Circle != nil {
		// A circle without a source-assigned number can only be recognized
		// within this work, so that is the scope its identity has.
		workScope := &target
		if group.Circle.SourceID != nil {
			workScope = nil
		}
		id, err := upsertEntity(
			tx,
			"circles",
			"source_circle_id",
			group.Source,
			group.Circle.SourceID,
			group.Circle.Name,
			workScope,
		)
		if err != nil {
			return err
		}
		circleID = &id
	}

	if _, err := tx.Exec(`
		UPDATE works
		SET title = ?, language = ?, circle_id = ?, nsfw = ?, age_category = ?, release_date = ?, source_url = ?,
		    cover_url = ?, thumbnail_url = ?, sample_cover_url = ?, series_id = ?, series_name = ?,
		    updated_at = ?
		WHERE id = ?
	`, nullableString(group.Title), nullableString(group.Language), circleID, nullableBool(group.NSFW), nullableInt(group.AgeCategory), nullableString(group.ReleaseDate),
		nullableString(group.SourceURL), nullableString(group.CoverURL), nullableString(group.ThumbnailURL),
		nullableString(group.SampleCoverURL), nullableString(group.SeriesID), nullableString(group.SeriesName),
		formatWorksTimestamp(now), string(target),
	); err != nil {
		return err
	}

	if err := replaceWorkEntities(
		tx,
		target,
		"work_voice_actors",
		"voice_actors",
		"source_voice_actor_id",
		group.Source,
		group.VoiceActors,
	); err != nil {
		return err
	}
	if err := replaceWorkEntities(
		tx,
		target,
		"work_tags",
		"tags",
		"source_tag_id",
		group.Source,
		group.Tags,
	); err != nil {
		return err
	}
	// Entities that exist only within this work and that nothing points at any
	// more would otherwise accumulate silently as the work is re-imported.
	if err := deleteScopedEntities(tx, target, "circles", "source_circle_id"); err != nil {
		return err
	}
	return touchSourceState(tx, target, works.GroupCore, group.Source, group.FetchedAt)
}

// replaceWorkEntities rewrites one many-to-many side of a work: the named
// entities are created or reused, then the work's join rows are replaced so the
// list is exactly what the submission named, in the order it named them.
func replaceWorkEntities(
	tx *sql.Tx,
	target works.WorkCode,
	joinTable, entityTable, entitySourceIDColumn string,
	source works.Source,
	entities []works.EntityInput,
) error {
	if _, err := tx.Exec("DELETE FROM "+joinTable+" WHERE work_id = ?", string(target)); err != nil {
		return err
	}
	// An upstream payload can name the same entity twice - the same credited
	// actor listed under two spellings of one number, say. The join table holds
	// one row per entity, so the second mention keeps the first one's position
	// rather than colliding with it.
	seen := map[string]struct{}{}
	position := 0
	for _, entity := range entities {
		workScope := &target
		if entity.SourceID != nil {
			workScope = nil
		}
		id, err := upsertEntity(tx, entityTable, entitySourceIDColumn, source, entity.SourceID, entity.Name, workScope)
		if err != nil {
			return err
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if _, err := tx.Exec(
			"INSERT INTO "+joinTable+" (work_id, "+entityIDColumn(entityTable)+", position) VALUES (?, ?, ?)",
			string(target), id, position,
		); err != nil {
			return err
		}
		position++
	}
	return deleteScopedEntities(tx, target, entityTable, entitySourceIDColumn)
}

// entityIDColumn names the entity foreign key of a join table.
func entityIDColumn(entityTable string) string {
	switch entityTable {
	case "voice_actors":
		return "voice_actor_id"
	default:
		return "tag_id"
	}
}

// upsertEntity finds or creates one source entity. Two identities are possible:
// the source's own number when it has one, and otherwise the (source, work,
// name) triple, which keeps the row's id stable across refreshes instead of
// minting a new one every time.
func upsertEntity(
	tx *sql.Tx,
	table, sourceIDColumn string,
	source works.Source,
	sourceID *string,
	name string,
	workScope *works.WorkCode,
) (string, error) {
	var (
		found string
		err   error
	)
	if sourceID != nil {
		err = tx.QueryRow(
			"SELECT id FROM "+table+" WHERE source = ? AND "+sourceIDColumn+" = ?",
			string(source), *sourceID,
		).Scan(&found)
	} else {
		var workID any
		if workScope != nil {
			workID = string(*workScope)
		}
		err = tx.QueryRow(
			"SELECT id FROM "+table+" WHERE source = ? AND "+sourceIDColumn+" IS NULL AND work_id IS ? AND name = ?",
			string(source), workID, name,
		).Scan(&found)
	}
	if err == nil {
		if sourceID != nil && (table == "tags" || table == "circles") {
			// Accepted snapshots refresh display names without changing entity identity.
			if _, updateErr := tx.Exec("UPDATE "+table+" SET name = ? WHERE id = ?", name, found); updateErr != nil {
				return "", updateErr
			}
		}
		return found, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	id := uuid.NewString()
	var workID any
	if workScope != nil {
		workID = string(*workScope)
	}
	if _, err := tx.Exec(
		"INSERT INTO "+table+" (id, source, "+sourceIDColumn+", name, work_id) VALUES (?, ?, ?, ?, ?)",
		id, string(source), nullableString(sourceID), name, workID,
	); err != nil {
		return "", err
	}
	return id, nil
}

// deleteScopedEntities removes the entities that exist only within this work
// and that no row points at any more.
func deleteScopedEntities(tx *sql.Tx, target works.WorkCode, table, sourceIDColumn string) error {
	var reference string
	switch table {
	case "circles":
		reference = "SELECT 1 FROM works w WHERE w.circle_id = " + table + ".id"
	case "voice_actors":
		reference = "SELECT 1 FROM work_voice_actors j WHERE j.voice_actor_id = " + table + ".id"
	default:
		reference = "SELECT 1 FROM work_tags j WHERE j.tag_id = " + table + ".id"
	}
	_, err := tx.Exec(
		"DELETE FROM "+table+" WHERE work_id = ? AND "+sourceIDColumn+" IS NULL AND NOT EXISTS ("+reference+")",
		string(target),
	)
	return err
}

// applyStatisticsGroup writes the sales figure. Sales have one source and one
// only, so the only question is whether this fetch is newer than the stored one.
func applyStatisticsGroup(
	tx *sql.Tx,
	target works.WorkCode,
	group *works.StatisticsGroup,
	stored map[works.DataGroup]works.SourceState,
	outcome *works.ImportOutcome,
) error {
	state, present := stored[works.GroupStatistics]
	decision := works.DecideGroup(
		works.GroupStatistics, works.StatisticsSource, group.FetchedAt.UnixNano(), state, present,
	)
	if !recordGroup(outcome, works.GroupStatistics, decision, works.AppliedGroup{
		DataGroup: works.GroupStatistics,
		DLCount:   group.DLCount,
	}) {
		return nil
	}
	if _, err := tx.Exec(`
		INSERT INTO work_statistics (work_id, dl_count) VALUES (?, ?)
		ON CONFLICT(work_id) DO UPDATE SET dl_count = excluded.dl_count
	`, string(target), nullableInt(group.DLCount)); err != nil {
		return err
	}
	return touchSourceState(tx, target, works.GroupStatistics, works.StatisticsSource, group.FetchedAt)
}

// applyEditionsGroup replaces the work's declared language editions.
func applyEditionsGroup(
	tx *sql.Tx,
	target works.WorkCode,
	group *works.EditionsGroup,
	stored map[works.DataGroup]works.SourceState,
	outcome *works.ImportOutcome,
	now time.Time,
) error {
	state, present := stored[works.GroupEditions]
	decision := works.DecideGroup(works.GroupEditions, group.Source, group.FetchedAt.UnixNano(), state, present)
	if !recordGroup(outcome, works.GroupEditions, decision, works.AppliedGroup{DataGroup: works.GroupEditions}) {
		return nil
	}
	if _, err := tx.Exec("DELETE FROM work_language_editions WHERE work_id = ?", string(target)); err != nil {
		return err
	}
	for _, edition := range group.Items {
		// The version root is named by the list, so it becomes a known work.
		if _, err := ensurePlaceholder(tx, edition.VersionRootRJ, outcome, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO work_language_editions
			    (work_id, version_root_rj, language, label, source_edition_id, display_order)
			VALUES (?, ?, ?, ?, ?, ?)
		`, string(target), string(edition.VersionRootRJ), edition.Language,
			nullableString(edition.Label), nullableString(edition.EditionID), edition.DisplayOrder,
		); err != nil {
			return err
		}
	}
	return touchSourceState(tx, target, works.GroupEditions, group.Source, group.FetchedAt)
}

// applyRelationsGroup replaces the work's own relation report and the child
// edges it declared, then recomputes every derived relation the change touches.
//
// The report is a snapshot, so a submission replaces both rows wholesale; a
// group that is not submitted leaves both untouched.
func applyRelationsGroup(
	tx *sql.Tx,
	target works.WorkCode,
	group *works.RelationsGroup,
	stored map[works.DataGroup]works.SourceState,
	outcome *works.ImportOutcome,
	now time.Time,
) ([]works.RelationWarning, error) {
	state, present := stored[works.GroupRelations]
	decision := works.DecideGroup(works.GroupRelations, group.Source, group.FetchedAt.UnixNano(), state, present)
	if !recordGroup(outcome, works.GroupRelations, decision, works.AppliedGroup{DataGroup: works.GroupRelations}) {
		return nil, nil
	}

	before, err := loadEvidence(tx)
	if err != nil {
		return nil, err
	}

	// Every work this report names has to exist before the edge rows can point
	// at it. A work named only by a relation is a placeholder: declared by the
	// source, never found on disk.
	for _, referenced := range referencedWorks(group) {
		if _, placeholderErr := ensurePlaceholder(tx, referenced, outcome, now); placeholderErr != nil {
			return nil, placeholderErr
		}
	}

	if _, execErr := tx.Exec("DELETE FROM work_relation_self WHERE work_id = ?", string(target)); execErr != nil {
		return nil, execErr
	}
	if _, execErr := tx.Exec(
		"DELETE FROM work_declared_children WHERE declaring_work_id = ?",
		string(target),
	); execErr != nil {
		return nil, execErr
	}
	if _, execErr := tx.Exec(`
		INSERT INTO work_relation_self (work_id, role, language, original_rj, parent_rj, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, string(target), string(group.Role), nullableString(group.Language),
		nullableCode(group.OriginalRJ), nullableCode(group.ParentRJ),
		string(group.Source), formatWorksTimestamp(group.FetchedAt),
	); execErr != nil {
		return nil, execErr
	}
	for _, child := range group.ChildRJs {
		if _, execErr := tx.Exec(`
			INSERT INTO work_declared_children (declaring_work_id, child_work_id, source, fetched_at)
			VALUES (?, ?, ?, ?)
		`, string(target), string(child), string(group.Source), formatWorksTimestamp(group.FetchedAt)); execErr != nil {
			return nil, execErr
		}
	}

	after, err := loadEvidence(tx)
	if err != nil {
		return nil, err
	}
	// The structural rules run against the catalog this submission leaves
	// behind, so a relation that only becomes illegal in combination with the
	// stored evidence is still refused - and the whole submission goes with it.
	if violation := works.ValidateRelations(after); violation != nil {
		return nil, works.Invalid(violation.Code, violation.Message)
	}

	affected := works.AffectedWorks(before, after, []works.WorkCode{target})
	if err := recomputeRelations(tx, after, affected); err != nil {
		return nil, err
	}
	if err := touchSourceState(tx, target, works.GroupRelations, group.Source, group.FetchedAt); err != nil {
		return nil, err
	}

	// Only the warnings this submission is responsible for: the ones touching
	// the work it named or a work the change reached.
	return warningsFor(works.Resolve(after).Warnings, affected), nil
}

// referencedWorks lists every work a relation report points at.
func referencedWorks(group *works.RelationsGroup) []works.WorkCode {
	referenced := make([]works.WorkCode, 0, len(group.ChildRJs)+2)
	if group.OriginalRJ != nil {
		referenced = append(referenced, *group.OriginalRJ)
	}
	if group.ParentRJ != nil {
		referenced = append(referenced, *group.ParentRJ)
	}
	referenced = append(referenced, group.ChildRJs...)
	return referenced
}

// warningsFor keeps the warnings about the works a change reached.
func warningsFor(warnings []works.RelationWarning, affected []works.WorkCode) []works.RelationWarning {
	inScope := make(map[works.WorkCode]struct{}, len(affected))
	for _, id := range affected {
		inScope[id] = struct{}{}
	}
	var kept []works.RelationWarning
	for _, warning := range warnings {
		if _, ok := inScope[warning.WorkID]; ok {
			kept = append(kept, warning)
		}
	}
	return kept
}

// ensurePlaceholder creates a work row for a number that was named but never
// found on disk. An existing row is left exactly as it is: a placeholder
// becomes an ordinary entry when a scan finds it, and nothing here undoes that.
func ensurePlaceholder(tx *sql.Tx, id works.WorkCode, outcome *works.ImportOutcome, now time.Time) (bool, error) {
	exists, err := workExists(tx, id)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := tx.Exec(
		"INSERT INTO works (id, created_at, updated_at) VALUES (?, ?, ?)",
		string(id), formatWorksTimestamp(now), formatWorksTimestamp(now),
	); err != nil {
		return false, err
	}
	outcome.CreatedPlaceholders = append(outcome.CreatedPlaceholders, id)
	return true, nil
}

// workExists reports whether a work row is present.
func workExists(tx *sql.Tx, id works.WorkCode) (bool, error) {
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM works WHERE id = ?", string(id)).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// loadEvidence reads both evidence tables: what works said about themselves and
// what child edges they declared.
func loadEvidence(tx *sql.Tx) (works.Evidence, error) {
	evidence := works.Evidence{
		Self:             map[works.WorkCode]works.RelationSelf{},
		DeclaredChildren: map[works.WorkCode][]works.WorkCode{},
	}

	selfRows, queryErr := tx.Query("SELECT work_id, role, language, original_rj, parent_rj FROM work_relation_self")
	if queryErr != nil {
		return evidence, queryErr
	}
	if rowsErr := func() error {
		defer selfRows.Close()
		for selfRows.Next() {
			var (
				id, role           string
				language, original sql.NullString
				parent             sql.NullString
			)
			if scanErr := selfRows.Scan(&id, &role, &language, &original, &parent); scanErr != nil {
				return scanErr
			}
			evidence.Self[works.WorkCode(id)] = works.RelationSelf{
				Role:       works.RelationRole(role),
				Language:   nullString(language),
				OriginalRJ: nullWorkCode(original),
				ParentRJ:   nullWorkCode(parent),
			}
		}
		return selfRows.Err()
	}(); rowsErr != nil {
		return evidence, rowsErr
	}

	childRows, err := tx.Query("SELECT declaring_work_id, child_work_id FROM work_declared_children")
	if err != nil {
		return evidence, err
	}
	defer childRows.Close()
	for childRows.Next() {
		var declaring, child string
		if err := childRows.Scan(&declaring, &child); err != nil {
			return evidence, err
		}
		evidence.DeclaredChildren[works.WorkCode(declaring)] = append(
			evidence.DeclaredChildren[works.WorkCode(declaring)], works.WorkCode(child),
		)
	}
	return evidence, childRows.Err()
}

// recomputeRelations rewrites the derived relation of every affected work from
// the evidence, deleting a row whose resolution says nothing at all - a work
// that gave up its parent has no derived values left to keep.
func recomputeRelations(tx *sql.Tx, evidence works.Evidence, affected []works.WorkCode) error {
	resolution := works.Resolve(evidence)
	for _, id := range affected {
		relation := resolution.Relations[id]
		if relation.IsEmpty() {
			if _, err := tx.Exec("DELETE FROM work_translations WHERE work_id = ?", string(id)); err != nil {
				return err
			}
			continue
		}
		var role any
		if relation.Role != nil {
			role = string(*relation.Role)
		}
		if _, err := tx.Exec(`
			INSERT INTO work_translations (work_id, role, language, original_work_id, parent_work_id)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(work_id) DO UPDATE SET
				role = excluded.role,
				language = excluded.language,
				original_work_id = excluded.original_work_id,
				parent_work_id = excluded.parent_work_id
		`, string(id), role, nullableString(relation.Language),
			nullableCode(relation.OriginalID), nullableCode(relation.ParentID),
		); err != nil {
			return err
		}
	}
	return nil
}

// ==================== nullable helpers ====================

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return boolToInt(*value)
}

func nullableCode(value *works.WorkCode) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
