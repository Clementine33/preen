package sqlite

import (
	"database/sql"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

// listeningStateSQL derives a work's listening state from its rows (w is the
// listing's works alias): any finished Rendition makes it finished, otherwise
// any position makes it listening.
const listeningStateSQL = `(CASE
	WHEN EXISTS (SELECT 1 FROM work_listening l WHERE l.work_id = w.id AND l.finished_at IS NOT NULL) THEN 'finished'
	WHEN EXISTS (SELECT 1 FROM work_listening l WHERE l.work_id = w.id AND l.track_key IS NOT NULL) THEN 'listening'
	ELSE 'none' END)`

const listeningColumns = `rendition, track_key, position_sec, track_index, track_count, finished_at, updated_at`

// PutListening upserts one Rendition's snapshot. A same-client write whose seq
// is not greater leaves the row (the conflict clause's WHERE); a later position
// write keeps finished_at, a finishing write clears the position.
func (r *Repository) PutListening(
	id works.WorkCode,
	req works.ListeningRequest,
	now time.Time,
) (works.Listening, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return works.Listening{}, err
	}
	defer tx.Rollback()

	if exists, existsErr := workExists(tx, id); existsErr != nil {
		return works.Listening{}, existsErr
	} else if !exists {
		return works.Listening{}, works.NotFound(works.CodeWorkNotFound, "no such work")
	}

	stamp := formatWorksTimestamp(now)
	var trackKey, position, index, count, finishedAt any
	if req.Finished {
		finishedAt = stamp
	} else {
		trackKey, position, index, count = req.TrackKey, req.PositionSec, req.TrackIndex, req.TrackCount
	}
	if _, err = tx.Exec(`
		INSERT INTO work_listening (work_id, rendition, track_key, position_sec, track_index, track_count,
		                            finished_at, client, seq, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (work_id, rendition) DO UPDATE SET
			track_key = excluded.track_key, position_sec = excluded.position_sec,
			track_index = excluded.track_index, track_count = excluded.track_count,
			finished_at = COALESCE(excluded.finished_at, work_listening.finished_at),
			client = excluded.client, seq = excluded.seq, updated_at = excluded.updated_at
		WHERE work_listening.client <> excluded.client OR excluded.seq > work_listening.seq`,
		string(id), req.Rendition, trackKey, position, index, count, finishedAt, req.Client, req.Seq, stamp,
	); err != nil {
		return works.Listening{}, err
	}
	row, err := scanListening(tx.QueryRow(
		"SELECT "+listeningColumns+" FROM work_listening WHERE work_id = ? AND rendition = ?",
		string(id), req.Rendition,
	))
	if err != nil {
		return works.Listening{}, err
	}
	return row, tx.Commit()
}

// ClearListening removes every row of the work; an unknown work is a 404.
func (r *Repository) ClearListening(id works.WorkCode) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireWork(tx, id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM work_listening WHERE work_id = ?", string(id)); err != nil {
		return err
	}
	return tx.Commit()
}

// scanListening reads listeningColumns; lead are extra columns selected before them.
func scanListening(row interface{ Scan(...any) error }, lead ...any) (works.Listening, error) {
	var (
		l               works.Listening
		key, finishedAt sql.NullString
		position        sql.NullFloat64
		index, count    sql.NullInt64
	)
	if err := row.Scan(
		append(lead, &l.Rendition, &key, &position, &index, &count, &finishedAt, &l.UpdatedAt)...); err != nil {
		return l, err
	}
	l.TrackKey, l.FinishedAt = nullString(key), nullString(finishedAt)
	l.TrackIndex, l.TrackCount = nullInt(index), nullInt(count)
	if position.Valid {
		l.PositionSec = &position.Float64
	}
	return l, nil
}

// loadListening reads the work's rows, newest first.
func loadListening(q workDetailReader, id works.WorkCode) ([]works.Listening, error) {
	rows, err := q.Query(
		"SELECT "+listeningColumns+" FROM work_listening WHERE work_id = ? ORDER BY updated_at DESC, rendition",
		string(id),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []works.Listening{}
	for rows.Next() {
		l, err := scanListening(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// listeningSummary digests rows (newest first) into the listing's summary.
func listeningSummary(rows []works.Listening) works.ListeningSummary {
	summary := works.ListeningSummary{State: works.ListeningNone}
	finished := false
	for _, l := range rows {
		finished = finished || l.FinishedAt != nil
		if l.TrackKey != nil && summary.UpdatedAt == nil {
			updated := l.UpdatedAt
			summary.TrackIndex, summary.TrackCount, summary.UpdatedAt = l.TrackIndex, l.TrackCount, &updated
			summary.State = works.ListeningActive
		}
	}
	if finished {
		summary.State = works.ListeningFinished
	}
	return summary
}

// attachListening fills the page's summaries with one query.
func (r *Repository) attachListening(summaries []works.WorkSummary) error {
	if len(summaries) == 0 {
		return nil
	}
	ids := make([]string, len(summaries))
	index := make(map[string]int, len(summaries))
	for i, summary := range summaries {
		ids[i] = string(summary.ID)
		index[ids[i]] = i
		summaries[i].Listening = listeningSummary(nil)
	}
	rows, err := r.db.Query(`
		SELECT work_id, `+listeningColumns+` FROM work_listening
		WHERE work_id IN (`+placeholderList(len(ids))+`) ORDER BY updated_at DESC, rendition`, textArgs(ids)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	perWork := map[string][]works.Listening{}
	for rows.Next() {
		var workID string
		l, err := scanListening(rows, &workID)
		if err != nil {
			return err
		}
		perWork[workID] = append(perWork[workID], l)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for workID, list := range perWork {
		summaries[index[workID]].Listening = listeningSummary(list)
	}
	return nil
}
