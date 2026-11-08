package sqlite

import (
	"encoding/json"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

// preferencesKey is the common_settings row holding the single user's
// preferences. It is not a Task kind, so the task-settings routes never reach it.
const preferencesKey = "preferences"

// Preferences returns the stored preferences, the zero value when never saved.
func (r *Repository) Preferences() (works.Preferences, error) {
	var prefs works.Preferences
	row, err := r.GetCommonSettings(preferencesKey)
	if err != nil || row == nil {
		return prefs, err
	}
	err = json.Unmarshal([]byte(row.Settings), &prefs)
	return prefs, err
}

// SavePreferences merges the update into the stored row as a JSON merge patch
// (RFC 7396): a missing field stays, a null field is removed, an array
// replaces. One upsert statement keeps concurrent writes of different fields
// from losing each other; the last write wins per field.
func (r *Repository) SavePreferences(update works.PreferencesUpdate, now time.Time) error {
	patch := map[string]any{}
	if update.TrackFormats != nil {
		patch["track_formats"] = *update.TrackFormats
	}
	if update.DefaultLibraryID.Set {
		patch["default_library_id"] = update.DefaultLibraryID.Value
	}
	payload, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO common_settings (operation_type, settings_json, updated_at)
		VALUES (?, json_patch('{}', ?), ?)
		ON CONFLICT(operation_type) DO UPDATE SET
			settings_json = json_patch(settings_json, ?),
			updated_at = excluded.updated_at
	`, preferencesKey, string(payload), now.Format(timeFormat), string(payload))
	return err
}
