package sqlite

import (
	"encoding/json"
	"time"
)

// preferencesKey is the common_settings row holding the single user's
// preferences. It is not a Task kind, so the task-settings routes never reach it.
const preferencesKey = "preferences"

type storedPreferences struct {
	TrackFormats []string `json:"track_formats"`
}

// TrackFormats returns the stored Format Preference, nil when never saved.
func (r *Repository) TrackFormats() ([]string, error) {
	row, err := r.GetCommonSettings(preferencesKey)
	if err != nil || row == nil {
		return nil, err
	}
	var stored storedPreferences
	if err := json.Unmarshal([]byte(row.Settings), &stored); err != nil {
		return nil, err
	}
	return stored.TrackFormats, nil
}

// SaveTrackFormats upserts the Format Preference; the last write wins.
func (r *Repository) SaveTrackFormats(formats []string, now time.Time) error {
	payload, err := json.Marshal(storedPreferences{TrackFormats: formats})
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO common_settings (operation_type, settings_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(operation_type) DO UPDATE SET
			settings_json = excluded.settings_json,
			updated_at = excluded.updated_at
	`, preferencesKey, string(payload), now.Format(timeFormat))
	return err
}
