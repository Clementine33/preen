package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/works"
)

// WorksRepository implements the catalog's storage port. The rules - what a
// submission may contain, which group supersedes which, what a change to one
// work implies for others - live in package works; this file is the SQL and the
// transaction boundary.
var _ works.Store = (*Repository)(nil)

// listProjection is the ordinary listing's fixed column set. The count of
// copies is a correlated subquery rather than a join, so a work with several
// copies stays one row. Legacy rows fall back to their resolved relation language.
var listProjection = `
	SELECT w.id, w.title, w.cover_url, c.name, COALESCE(w.language, t.language), w.nsfw, w.age_category, w.release_date,
	       s.dl_count, u.rating, EXISTS (SELECT 1 FROM collection_works f WHERE f.collection_id = 'favorites' AND f.work_id = w.id),
	       CASE WHEN COALESCE(u.note, '') <> '' THEN 1 ELSE 0 END,
	       w.collected_at,
	       (SELECT COUNT(*) FROM work_copies wc WHERE wc.work_id = w.id)` + listFrom

// placeholderList renders "?, ?, ?" for an IN list.
func placeholderList(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// textArgs converts ids into the arguments of an IN list.
func textArgs(ids []string) []any {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	return args
}

// listFrom is the FROM and WHERE head every listing query shares: the page,
// its count and the facet counts read the same joins, so a filter fragment
// that names t (translations) or u (user data) means the same thing in each.
const listFrom = `
	FROM works w
	LEFT JOIN circles c ON c.id = w.circle_id
	LEFT JOIN work_translations t ON t.work_id = w.id
	LEFT JOIN work_statistics s ON s.work_id = w.id
	LEFT JOIN work_user_data u ON u.work_id = w.id
	WHERE 1 = 1`

// whereTail renders the filter's fragments after the shared head.
func (b *listQueryBuilder) whereTail() string {
	if len(b.where) == 0 {
		return ""
	}
	return "\n  AND " + strings.Join(b.where, "\n  AND ")
}

// listingSQL renders one page of the ordinary listing: the filter's fragments,
// the ordering and the page window. The ordering is total, so consecutive
// pages neither repeat nor skip a row.
func listingSQL(filter works.Filter) (string, []any) {
	builder := listQueryBuilder{}
	builder.applyFilters(filter)
	query := listProjection + builder.whereTail() +
		"\n  ORDER BY " + orderClause(filter) + "\n  LIMIT ? OFFSET ?"
	return query, append(builder.args, filter.Limit, (filter.Page-1)*filter.Limit)
}

// ListWorks returns one page of the ordinary listing and the number of works
// the whole filter lists. Filters and ordering are resolved in SQL, and only
// the page's works have their tags fetched, so the query count does not grow
// with the page size.
func (r *Repository) ListWorks(filter works.Filter) (works.Page, error) {
	// The service validates the filter and fills the defaults before this is
	// reached; the guards keep a direct caller from turning an unset page or
	// page size into a negative window.
	if filter.Limit <= 0 {
		filter.Limit = works.DefaultLimit
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}

	page := works.Page{}
	builder := listQueryBuilder{}
	builder.applyFilters(filter)
	if err := r.db.QueryRow("SELECT COUNT(*)"+listFrom+builder.whereTail(), builder.args...).
		Scan(&page.Total); err != nil {
		return works.Page{}, err
	}

	query, args := listingSQL(filter)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return works.Page{}, err
	}
	page.Works, err = r.readWorkSummaries(rows)
	return page, err
}

// readWorkSummaries shares the catalog projection with collection detail pages.
func (r *Repository) readWorkSummaries(rows *sql.Rows) ([]works.WorkSummary, error) {
	defer rows.Close()
	summaries := []works.WorkSummary{}

	for rows.Next() {
		var (
			summary     works.WorkSummary
			id          string
			title       sql.NullString
			coverURL    sql.NullString
			circleName  sql.NullString
			language    sql.NullString
			nsfw        sql.NullInt64
			ageCategory sql.NullInt64
			releaseDate sql.NullString
			dlCount     sql.NullInt64
			rating      sql.NullInt64
			favorite    int
			hasNote     int
			collectedAt sql.NullString
			copyCount   int
		)
		if scanErr := rows.Scan(
			&id, &title, &coverURL, &circleName, &language, &nsfw, &ageCategory, &releaseDate,
			&dlCount, &rating, &favorite, &hasNote, &collectedAt, &copyCount,
		); scanErr != nil {
			return nil, scanErr
		}
		summary.ID = works.WorkCode(id)
		summary.Title = nullString(title)
		summary.CoverURL = nullString(coverURL)
		summary.CircleName = nullString(circleName)
		summary.Language = nullString(language)
		summary.NSFW = nullBool(nsfw)
		summary.AgeCategory = nullInt(ageCategory)
		summary.ReleaseDate = nullString(releaseDate)
		summary.DLCount = nullInt(dlCount)
		summary.Rating = nullInt(rating)
		summary.Favorite = favorite == 1
		summary.HasNote = hasNote == 1
		summary.CollectedAt = collectedAt.String
		summary.CopyCount = copyCount
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.attachUserTags(summaries); err != nil {
		return nil, err
	}
	if err := r.attachListening(summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

// orderClause renders the ordering. The expression is chosen here from a
// closed set, never built from the request, and the trailing work number keeps
// the order total when two rows share a sort value.
func orderClause(filter works.Filter) string {
	return sortKeys(filter.Sort, filter.Direction) + ", w.id ASC"
}

// sortKeys renders the sort field's order-by keys, without a tie-break. The
// collection detail shares it and breaks ties by saved position instead.
func sortKeys(field works.SortField, dir works.Direction) string {
	direction := "ASC"
	if dir == works.Descending {
		direction = "DESC"
	}
	if column, ok := nullableSortColumn(field); ok {
		// Unknown values are always last, in either direction, which is why the
		// null flag leads the ordering and never flips with it.
		return "(" + column + " IS NULL) ASC, " + column + " " + direction
	}
	if field == works.SortByCollectedAt {
		return "w.collected_at " + direction
	}
	// The work number, and the fallback: Validate refuses any other field.
	return "w.id " + direction
}

// nullableSortColumn is the column of an order whose value can be unknown.
// The rating lives in the user-data table, which is why its order is sorted by
// the planner rather than read from an index (ADR 0009).
func nullableSortColumn(field works.SortField) (string, bool) {
	switch field {
	case works.SortByReleaseDate:
		return "w.release_date", true
	case works.SortByRating:
		return "u.rating", true
	case works.SortByListenedAt:
		return "(SELECT MAX(l.updated_at) FROM work_listening l WHERE l.work_id = w.id)", true
	case works.SortByWorkID, works.SortByCollectedAt:
	}
	return "", false
}

// listQueryBuilder accumulates the WHERE fragments and their bound values.
type listQueryBuilder struct {
	where []string
	args  []any
}

func (b *listQueryBuilder) add(fragment string, args ...any) {
	b.where = append(b.where, fragment)
	b.args = append(b.args, args...)
}

// applyFilters turns the filter contract into SQL. Different dimensions
// intersect; inside the rating dimension a range and the not-rated switch
// union, so asking for "8 and up, or not rated" is one predicate rather than
// two that contradict each other.
func (b *listQueryBuilder) applyFilters(filter works.Filter) {
	if filter.Query != nil {
		b.addQuery(*filter.Query)
	}
	if len(filter.Circles) > 0 {
		b.add("w.circle_id IN ("+placeholderList(len(filter.Circles))+")", textArgs(filter.Circles)...)
	}
	b.addSet("work_voice_actors", "voice_actor_id", filter.VoiceActors, filter.VoiceActorMatch)
	b.addSet("work_tags", "tag_id", filter.SourceTags, filter.SourceTagMatch)
	b.addSet("work_user_tags", "tag_id", filter.UserTags, filter.UserTagMatch)
	b.addTagGroups(filter.TagGroups, filter.TagMatch)

	if len(filter.Languages) > 0 {
		// A work that reported no language cannot match a language filter: the
		// dimension asks about a value, and unknown is not one of the values.
		b.add(
			"COALESCE(w.language, t.language) IN ("+placeholderList(len(filter.Languages))+")",
			textArgs(filter.Languages)...,
		)
	}
	if filter.AgeCategory != nil {
		b.add("w.age_category = ?", *filter.AgeCategory)
	}
	switch filter.NSFW {
	case works.NSFWAny:
		// No constraint: every value of the flag is wanted.
	case works.NSFWOnly:
		b.add("w.nsfw = 1")
	case works.NSFWExcluded:
		b.add("w.nsfw = 0")
	case works.NSFWUnknown:
		b.add("w.nsfw IS NULL")
	}
	switch {
	case filter.ReleasedFrom != nil && filter.ReleasedTo != nil:
		b.add("w.release_date BETWEEN ? AND ?", *filter.ReleasedFrom, *filter.ReleasedTo)
	case filter.ReleasedFrom != nil:
		b.add("w.release_date >= ?", *filter.ReleasedFrom)
	case filter.ReleasedTo != nil:
		b.add("w.release_date <= ?", *filter.ReleasedTo)
	}

	unrated := filter.Unrated != nil && *filter.Unrated
	hasRange := filter.RatingFrom != nil || filter.RatingTo != nil
	switch {
	case hasRange && unrated:
		rangeClause, rangeArgs := ratingRange(filter)
		b.add("("+rangeClause+" OR u.rating IS NULL)", rangeArgs...)
	case hasRange:
		rangeClause, rangeArgs := ratingRange(filter)
		b.add("(u.rating IS NOT NULL AND "+rangeClause+")", rangeArgs...)
	case filter.Unrated != nil && unrated:
		b.add("u.rating IS NULL")
	case filter.Unrated != nil:
		b.add("u.rating IS NOT NULL")
	}

	if filter.Favorite != nil {
		b.add(
			"EXISTS (SELECT 1 FROM collection_works f WHERE f.collection_id = 'favorites' AND f.work_id = w.id) = ?",
			boolToInt(*filter.Favorite),
		)
	}
	b.addCopyFilter(filter)
	if len(filter.Listening) > 0 {
		b.add(listeningStateSQL+" IN ("+placeholderList(len(filter.Listening))+")", textArgs(filter.Listening)...)
	}
}

// addQuery matches the literal text against the number and the title. `%` and
// `_` are ordinary characters here: instr is a substring search, where LIKE
// would read them as wildcards.
func (b *listQueryBuilder) addQuery(query string) {
	if code, ok := works.ParseWorkCode(query); ok {
		// A query that is itself a number matches that work exactly, so the
		// short spelling finds the row stored in the canonical one.
		b.add(`(w.id = ? OR
		    (t.parent_work_id = ? AND t.language IN ('CHI_HANS', 'CHI_HANT') AND
		     EXISTS (SELECT 1 FROM work_copies lookup_copy WHERE lookup_copy.work_id = w.id)))`,
			string(code), string(code))
		return
	}
	b.add("(instr(w.id, ?) > 0 OR instr(COALESCE(w.title, ''), ?) > 0)", query, query)
}

// addSet adds one many-to-many filter. "all" is a conjunction of exists
// clauses, "any" a single one: the modes answer different questions and the
// caller has to say which it means.
func (b *listQueryBuilder) addSet(table, column string, ids []string, mode works.MatchMode) {
	if len(ids) == 0 {
		return
	}
	if mode == works.MatchAll {
		for _, id := range ids {
			b.add("EXISTS (SELECT 1 FROM "+table+" m WHERE m.work_id = w.id AND m."+column+" = ?)", id)
		}
		return
	}
	b.add(
		"EXISTS (SELECT 1 FROM "+table+" m WHERE m.work_id = w.id AND m."+column+" IN ("+
			placeholderList(len(ids))+"))",
		textArgs(ids)...,
	)
}

// addTagGroups matches the combined tag filter. One group is one picked tag:
// the work carries any of its ids, in either vocabulary. All groups must hold
// under MatchAll; under MatchAny one is enough, so the groups form a single
// OR'd fragment instead of intersecting like the other dimensions.
func (b *listQueryBuilder) addTagGroups(groups []works.TagGroup, mode works.MatchMode) {
	if len(groups) == 0 {
		return
	}
	fragments := make([]string, 0, len(groups))
	var args []any
	for _, group := range groups {
		var source, user []string
		for _, ref := range group {
			if ref.Kind == works.TagUser {
				user = append(user, ref.ID)
			} else {
				source = append(source, ref.ID)
			}
		}
		var sides []string
		for _, side := range []struct {
			table string
			ids   []string
		}{{"work_tags", source}, {"work_user_tags", user}} {
			if len(side.ids) == 0 {
				continue
			}
			sides = append(sides, "EXISTS (SELECT 1 FROM "+side.table+
				" m WHERE m.work_id = w.id AND m.tag_id IN ("+placeholderList(len(side.ids))+"))")
			args = append(args, textArgs(side.ids)...)
		}
		fragment := "(" + strings.Join(sides, " OR ") + ")"
		if mode == works.MatchAll {
			b.add(fragment, args...)
			args = nil
			continue
		}
		fragments = append(fragments, fragment)
	}
	if mode != works.MatchAll {
		b.add("("+strings.Join(fragments, " OR ")+")", args...)
	}
}

// addCopyFilter answers "does this work have a copy" - in the named libraries
// when some are named, in any library otherwise.
func (b *listQueryBuilder) addCopyFilter(filter works.Filter) {
	if len(filter.Libraries) == 0 && filter.HasCopy == nil {
		return
	}
	clause := "SELECT 1 FROM work_copies wc WHERE wc.work_id = w.id"
	args := []any{}
	if len(filter.Libraries) > 0 {
		clause += " AND wc.library_id IN (" + placeholderList(len(filter.Libraries)) + ")"
		args = textArgs(filter.Libraries)
	}
	if filter.HasCopy != nil && !*filter.HasCopy {
		b.add("NOT EXISTS ("+clause+")", args...)
		return
	}
	b.add("EXISTS ("+clause+")", args...)
}

// ratingRange renders the inclusive rating bounds that are present.
func ratingRange(filter works.Filter) (string, []any) {
	switch {
	case filter.RatingFrom != nil && filter.RatingTo != nil:
		return "u.rating BETWEEN ? AND ?", []any{*filter.RatingFrom, *filter.RatingTo}
	case filter.RatingFrom != nil:
		return "u.rating >= ?", []any{*filter.RatingFrom}
	default:
		return "u.rating <= ?", []any{*filter.RatingTo}
	}
}

// attachUserTags fetches the user tags of one page's works in a single query.
// Asking per work would make the request's cost grow with the page size.
func (r *Repository) attachUserTags(summaries []works.WorkSummary) error {
	if len(summaries) == 0 {
		return nil
	}
	ids := make([]string, 0, len(summaries))
	index := make(map[string]int, len(summaries))
	for i, summary := range summaries {
		ids = append(ids, string(summary.ID))
		index[string(summary.ID)] = i
	}
	rows, err := r.db.Query(`
		SELECT wt.work_id, ut.id, ut.name
		FROM work_user_tags wt
		JOIN user_tags ut ON ut.id = wt.tag_id
		WHERE wt.work_id IN (`+placeholderList(len(ids))+`)
		ORDER BY ut.name
	`, textArgs(ids)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var workID string
		var tag works.UserTag
		if err := rows.Scan(&workID, &tag.ID, &tag.Name); err != nil {
			return err
		}
		if at, ok := index[workID]; ok {
			summaries[at].UserTags = append(summaries[at].UserTags, tag)
		}
	}
	return rows.Err()
}

// GetWork loads the complete detail in one deferred read-only snapshot.
func (r *Repository) GetWork(id works.WorkCode) (*works.WorkDetail, bool, error) {
	tx, err := r.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	detail := &works.WorkDetail{}
	var (
		title, circleName           sql.NullString
		nsfw, favorite, ageCategory sql.NullInt64
		releaseDate                 sql.NullString
		dlCount, rating             sql.NullInt64
		collectedAt                 sql.NullString
		sourceURL, coverURL         sql.NullString
		sampleCoverURL              sql.NullString
		seriesID, seriesName        sql.NullString
		language, note              sql.NullString
		copyCount                   int
	)
	err = tx.QueryRow(`
		SELECT w.title, c.name, w.nsfw, w.age_category, w.release_date, w.source_url,
		       w.cover_url, w.sample_cover_url, w.series_id, w.series_name,
		       w.collected_at, s.dl_count, u.rating, EXISTS (SELECT 1 FROM collection_works f WHERE f.collection_id = 'favorites' AND f.work_id = w.id), u.note, COALESCE(w.language, t.language),
		       (SELECT COUNT(*) FROM work_copies wc WHERE wc.work_id = w.id)
		FROM works w
		LEFT JOIN circles c ON c.id = w.circle_id
		LEFT JOIN work_translations t ON t.work_id = w.id
		LEFT JOIN work_statistics s ON s.work_id = w.id
		LEFT JOIN work_user_data u ON u.work_id = w.id
		WHERE w.id = ?
	`, string(id)).Scan(
		&title, &circleName, &nsfw, &ageCategory, &releaseDate, &sourceURL,
		&coverURL, &sampleCoverURL, &seriesID, &seriesName,
		&collectedAt, &dlCount, &rating, &favorite, &note, &language, &copyCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	detail.ID = id
	detail.Title = nullString(title)
	detail.CircleName = nullString(circleName)
	detail.NSFW = nullBool(nsfw)
	detail.AgeCategory = nullInt(ageCategory)
	detail.ReleaseDate = nullString(releaseDate)
	detail.SourceURL = nullString(sourceURL)
	detail.CoverURL = nullString(coverURL)
	detail.SampleCoverURL = nullString(sampleCoverURL)
	detail.SeriesID = nullString(seriesID)
	detail.SeriesName = nullString(seriesName)
	detail.Language = nullString(language)
	detail.DLCount = nullInt(dlCount)
	detail.Rating = nullInt(rating)
	detail.Favorite = favorite.Int64 == 1
	detail.Note = note.String
	detail.HasNote = strings.TrimSpace(note.String) != ""
	detail.CollectedAt = collectedAt.String
	detail.CopyCount = copyCount

	if detail.VoiceActors, err = loadEntities(tx, id, "work_voice_actors", "voice_actors"); err != nil {
		return nil, false, err
	}
	if detail.SourceTags, err = loadEntities(tx, id, "work_tags", "tags"); err != nil {
		return nil, false, err
	}
	if detail.UserTags, err = loadUserTags(tx, id); err != nil {
		return nil, false, err
	}
	if detail.Editions, err = loadEditions(tx, id); err != nil {
		return nil, false, err
	}
	if detail.Copies, err = loadCopies(tx, id); err != nil {
		return nil, false, err
	}
	if detail.Relation, err = loadRelation(tx, id); err != nil {
		return nil, false, err
	}
	if err = loadRelationDetail(tx, detail); err != nil {
		return nil, false, err
	}
	if detail.CollectionIDs, err = loadUserCollectionIDs(tx, id); err != nil {
		return nil, false, err
	}
	if detail.Listening, err = loadListening(tx, id); err != nil {
		return nil, false, err
	}
	covers, err := loadLocalCovers(context.Background(), tx, []works.WorkCode{id})
	if err != nil {
		return nil, false, err
	}
	if cover, ok := covers[id]; ok {
		detail.LocalCover = &cover
	}
	detail.WorkSummary.Listening = listeningSummary(detail.Listening)
	return detail, true, nil
}

// loadUserCollectionIDs lists the user collections holding a work in the
// collection list's order (creation timestamp, then ID).
func loadUserCollectionIDs(q workDetailReader, id works.WorkCode) ([]string, error) {
	rows, err := q.Query(`
		SELECT c.id FROM collections c
		JOIN collection_works m ON m.collection_id = c.id
		WHERE m.work_id = ? AND c.id <> 'favorites'
		ORDER BY c.created_at, c.id
	`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		ids = append(ids, cid)
	}
	return ids, rows.Err()
}

// loadEntities reads one many-to-many side of a work in display order.
func loadEntities(q workDetailReader, id works.WorkCode, joinTable, entityTable string) ([]works.SourceEntity, error) {
	rows, err := q.Query(`
		SELECT e.id, e.source, e.source_`+strings.TrimSuffix(entityTable, "s")+`_id, e.name, e.work_id
		FROM `+joinTable+` j
		JOIN `+entityTable+` e ON e.id = j.`+strings.TrimSuffix(entityTable, "s")+`_id
		WHERE j.work_id = ?
		ORDER BY j.position, e.name
	`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []works.SourceEntity
	for rows.Next() {
		entity, scanErr := scanSourceEntity(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, entity)
	}
	return out, rows.Err()
}

// loadUserTags reads the tags the user attached to one work.
func loadUserTags(q workDetailReader, id works.WorkCode) ([]works.UserTag, error) {
	rows, err := q.Query(`
		SELECT ut.id, ut.name
		FROM work_user_tags wt
		JOIN user_tags ut ON ut.id = wt.tag_id
		WHERE wt.work_id = ?
		ORDER BY ut.name
	`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []works.UserTag
	for rows.Next() {
		var tag works.UserTag
		if scanErr := rows.Scan(&tag.ID, &tag.Name); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

// loadEditions reads the language editions a work declared.
func loadEditions(q workDetailReader, id works.WorkCode) ([]works.LanguageEdition, error) {
	rows, err := q.Query(`
		SELECT version_root_rj, language, label, source_edition_id, display_order
		FROM work_language_editions
		WHERE work_id = ?
		ORDER BY display_order, language
	`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []works.LanguageEdition
	for rows.Next() {
		var (
			edition   works.LanguageEdition
			root      string
			label     sql.NullString
			editionID sql.NullString
		)
		if scanErr := rows.Scan(&root, &edition.Language, &label, &editionID, &edition.DisplayOrder); scanErr != nil {
			return nil, scanErr
		}
		edition.VersionRootRJ = works.WorkCode(root)
		edition.Label = nullString(label)
		edition.EditionID = nullString(editionID)
		out = append(out, edition)
	}
	return out, rows.Err()
}

// loadCopies reads the directories bound to one work.
func loadCopies(q workDetailReader, id works.WorkCode) ([]works.Copy, error) {
	rows, err := q.Query(`
		SELECT c.id, c.library_id, c.rel_path, l.root_path FROM work_copies c JOIN libraries l ON l.id = c.library_id WHERE c.work_id = ? ORDER BY c.rel_path
	`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []works.Copy
	for rows.Next() {
		var bound works.Copy
		if scanErr := rows.Scan(&bound.ID, &bound.LibraryID, &bound.RelPath, &bound.LibraryRoot); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, bound)
	}
	return out, rows.Err()
}

// loadRelation reads the resolved relation, which the import rules keep in
// step with the evidence.
func loadRelation(q workDetailReader, id works.WorkCode) (works.ResolvedRelation, error) {
	var (
		role       sql.NullString
		language   sql.NullString
		originalID sql.NullString
		parentID   sql.NullString
	)
	err := q.QueryRow(`
		SELECT role, language, original_work_id, parent_work_id
		FROM work_translations WHERE work_id = ?
	`, string(id)).Scan(&role, &language, &originalID, &parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return works.ResolvedRelation{}, nil
	}
	if err != nil {
		return works.ResolvedRelation{}, err
	}
	var relation works.ResolvedRelation
	if role.Valid {
		value := works.RelationRole(role.String)
		relation.Role = &value
	}
	relation.Language = nullString(language)
	relation.OriginalID = nullWorkCode(originalID)
	relation.ParentID = nullWorkCode(parentID)
	return relation, nil
}

// scanSourceEntity reads one entity row from a stored source table.
func scanSourceEntity(rows *sql.Rows) (works.SourceEntity, error) {
	var (
		entity   works.SourceEntity
		source   string
		sourceID sql.NullString
		workID   sql.NullString
	)
	if err := rows.Scan(&entity.ID, &source, &sourceID, &entity.Name, &workID); err != nil {
		return entity, err
	}
	entity.Source = works.Source(source)
	entity.SourceID = nullString(sourceID)
	entity.WorkID = nullWorkCode(workID)
	return entity, nil
}

// PatchUserData applies a partial update. The row is created on first write, so
// a work with no user data has no row and reads as its zero value.
func (r *Repository) PatchUserData(
	id works.WorkCode,
	patch works.UserDataPatch,
	now time.Time,
) (works.UserData, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return works.UserData{}, err
	}
	defer tx.Rollback()

	if exists, existsErr := workExists(tx, id); existsErr != nil {
		return works.UserData{}, existsErr
	} else if !exists {
		return works.UserData{}, works.NotFound(works.CodeWorkNotFound, "no such work")
	}

	if _, insertErr := tx.Exec(
		"INSERT OR IGNORE INTO work_user_data (work_id, updated_at) VALUES (?, '')", string(id),
	); insertErr != nil {
		return works.UserData{}, insertErr
	}

	sets := []string{"updated_at = ?"}
	args := []any{formatWorksTimestamp(now)}
	if patch.Rating.Set {
		sets = append(sets, "rating = ?")
		if patch.Rating.Value == nil {
			args = append(args, nil)
		} else {
			args = append(args, *patch.Rating.Value)
		}
	}
	if patch.Favorite.Set {
		if patch.Favorite.Value {
			_, err = appendCollectionWorks(context.Background(), tx, collections.FavoritesID, []works.WorkCode{id}, now)
		} else {
			err = removeCollectionWork(context.Background(), tx, collections.FavoritesID, id, now)
		}
		if err != nil {
			return works.UserData{}, err
		}
	}
	if patch.Note.Set {
		sets = append(sets, "note = ?")
		args = append(args, patch.Note.Value)
	}
	args = append(args, string(id))
	if _, err := tx.Exec(
		"UPDATE work_user_data SET "+strings.Join(sets, ", ")+" WHERE work_id = ?", args...,
	); err != nil {
		return works.UserData{}, err
	}

	// An empty row carries no data; it is dropped in the same transaction.
	if _, err := tx.Exec(
		"DELETE FROM work_user_data WHERE work_id = ? AND rating IS NULL AND note = ''", string(id),
	); err != nil {
		return works.UserData{}, err
	}

	var (
		rating   sql.NullInt64
		favorite int
		note     string
	)
	if err := tx.QueryRow(
		`SELECT d.rating, EXISTS (SELECT 1 FROM collection_works f
		 WHERE f.collection_id = 'favorites' AND f.work_id = w.id), COALESCE(d.note, '')
		 FROM works w LEFT JOIN work_user_data d ON d.work_id = w.id WHERE w.id = ?`, string(id),
	).Scan(&rating, &favorite, &note); err != nil {
		return works.UserData{}, err
	}
	if err := tx.Commit(); err != nil {
		return works.UserData{}, err
	}
	if patch.Favorite.Set {
		r.catalogChanged()
	}
	return works.UserData{Rating: nullInt(rating), Favorite: favorite == 1, Note: note}, nil
}

// ReplaceUserTags points a work at exactly the given tags. Tags the request did
// not name are detached; the tags themselves are untouched.
func (r *Repository) ReplaceUserTags(id works.WorkCode, tagIDs []string) ([]works.UserTag, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM works WHERE id = ?", string(id)).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, works.NotFound(works.CodeWorkNotFound, "no such work")
	}

	if _, err := tx.Exec("DELETE FROM work_user_tags WHERE work_id = ?", string(id)); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, tagID := range tagIDs {
		if _, dup := seen[tagID]; dup {
			continue
		}
		seen[tagID] = struct{}{}
		var known int
		if err := tx.QueryRow("SELECT COUNT(*) FROM user_tags WHERE id = ?", tagID).Scan(&known); err != nil {
			return nil, err
		}
		if known == 0 {
			return nil, works.NotFound(works.CodeUserTagNotFound, "no such user tag")
		}
		if _, err := tx.Exec(
			"INSERT INTO work_user_tags (work_id, tag_id) VALUES (?, ?)", string(id), tagID,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return loadUserTags(r.db, id)
}

// ListUserTags returns every tag, ordered by name.
func (r *Repository) ListUserTags() ([]works.UserTag, error) {
	rows, err := r.db.Query("SELECT id, name FROM user_tags ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []works.UserTag
	for rows.Next() {
		var tag works.UserTag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

// CreateUserTag stores a tag unless one with the same name exists, in which
// case that one is returned: creating by name is idempotent, so a caller never
// has to look a tag up first.
func (r *Repository) CreateUserTag(name string) (works.UserTag, bool, error) {
	var existing works.UserTag
	err := r.db.QueryRow("SELECT id, name FROM user_tags WHERE name = ?", name).Scan(&existing.ID, &existing.Name)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return works.UserTag{}, false, err
	}
	tag := works.UserTag{ID: uuid.NewString(), Name: name}
	if _, err := r.db.Exec("INSERT INTO user_tags (id, name) VALUES (?, ?)", tag.ID, tag.Name); err != nil {
		return works.UserTag{}, false, err
	}
	return tag, true, nil
}

// RenameUserTag renames one tag. The name is unique, so a collision is a
// conflict rather than a silent merge.
func (r *Repository) RenameUserTag(id, name string) (works.UserTag, error) {
	result, err := r.db.Exec("UPDATE user_tags SET name = ? WHERE id = ?", name, id)
	if err != nil {
		if isUniqueConstraintError(err) {
			return works.UserTag{}, works.NewError(
				works.KindConflict,
				works.CodeUserTagExists,
				"another tag already has this name",
			)
		}
		return works.UserTag{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return works.UserTag{}, err
	}
	if affected == 0 {
		return works.UserTag{}, works.NotFound(works.CodeUserTagNotFound, "no such user tag")
	}
	return works.UserTag{ID: id, Name: name}, nil
}

// DeleteUserTag removes a tag; its relations go with it and no work is touched.
func (r *Repository) DeleteUserTag(id string) error {
	result, err := r.db.Exec("DELETE FROM user_tags WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return works.NotFound(works.CodeUserTagNotFound, "no such user tag")
	}
	return nil
}

// DeleteWork removes only catalog-owned records. Independent source reports
// and other works' derived external targets survive catalog membership changes.
func (r *Repository) DeleteWork(id works.WorkCode) *works.Error {
	tx, err := r.db.Begin()
	if err != nil {
		return internalError(err)
	}
	defer tx.Rollback()

	var exists int
	if scanErr := tx.QueryRow("SELECT COUNT(*) FROM works WHERE id = ?", string(id)).Scan(&exists); scanErr != nil {
		return internalError(scanErr)
	}
	if exists == 0 {
		return works.NotFound(works.CodeWorkNotFound, "no such work")
	}

	blockers, err := collectBlockers(tx, id)
	if err != nil {
		return internalError(err)
	}
	if len(blockers) > 0 {
		inUse := works.NewError(works.KindConflict, works.CodeWorkInUse, "this work is still referenced")
		inUse.Details = blockers
		return inUse
	}

	if _, err := tx.Exec("DELETE FROM works WHERE id = ?", string(id)); err != nil {
		return internalError(err)
	}
	if err := tx.Commit(); err != nil {
		return internalError(err)
	}
	r.catalogChanged()
	return nil
}

// collectBlockers checks stored copy bindings and personal data
// in the deleting write transaction; availability on disk never affects eligibility.
func collectBlockers(tx *sql.Tx, id works.WorkCode) ([]string, error) {
	blockers := []string{}
	var rating, note, tags bool
	if err := tx.QueryRow(`SELECT
	 EXISTS (SELECT 1 FROM work_user_data WHERE work_id = ? AND rating IS NOT NULL),
	 EXISTS (SELECT 1 FROM work_user_data WHERE work_id = ? AND note <> ''),
	 EXISTS (SELECT 1 FROM work_user_tags WHERE work_id = ?)`,
		string(id), string(id), string(id)).Scan(&rating, &note, &tags); err != nil {
		return nil, err
	}
	if rating {
		blockers = append(blockers, "a personal rating is saved for this work")
	}
	if note {
		blockers = append(blockers, "a personal note is saved for this work")
	}
	if tags {
		blockers = append(blockers, "user tags are attached to this work")
	}

	var copies int
	if err := tx.QueryRow("SELECT COUNT(*) FROM work_copies WHERE work_id = ?", string(id)).Scan(&copies); err != nil {
		return nil, err
	}
	if copies > 0 {
		blockers = append(blockers, "a local copy is bound to this work")
	}

	collectionRefs, err := collectionBlockers(tx, id)
	return append(blockers, collectionRefs...), err
}

// ==================== shared row helpers ====================

// requireWork checks existence while the caller holds the write transaction.
func requireWork(tx *sql.Tx, id works.WorkCode) error {
	var exists bool
	if err := tx.QueryRow("SELECT EXISTS (SELECT 1 FROM works WHERE id = ?)", string(id)).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return works.NotFound(works.CodeWorkNotFound, "no such work")
	}
	return nil
}

// formatWorksTimestamp renders a time in the catalog's fixed-width UTC form.
func formatWorksTimestamp(t time.Time) string {
	return t.UTC().Format(works.TimestampFormat)
}

// internalError wraps an unexpected storage failure as a refusal, keeping the
// driver's own message for the log.
func internalError(err error) *works.Error {
	wrapped := works.NewError(works.KindInternal, "INTERNAL", "the catalog could not complete the request")
	wrapped.Details = []string{err.Error()}
	return wrapped
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
}

func nullInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int64)
	return &out
}

func nullBool(value sql.NullInt64) *bool {
	if !value.Valid {
		return nil
	}
	out := value.Int64 == 1
	return &out
}

func nullWorkCode(value sql.NullString) *works.WorkCode {
	if !value.Valid {
		return nil
	}
	code := works.WorkCode(value.String)
	return &code
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
