package sqlite

import (
	"encoding/json"
	"time"
)

// upstreamProxyKey is the common_settings row holding the Upstream Proxy URL.
// Like preferences it is not a Task kind.
const upstreamProxyKey = "upstream_proxy"

type storedUpstreamProxy struct {
	URL string `json:"url"`
}

// UpstreamProxy returns the stored Upstream Proxy URL, "" when never saved. A
// row that cannot be decoded is an error: callers must not treat it as direct.
func (r *Repository) UpstreamProxy() (string, error) {
	row, err := r.GetCommonSettings(upstreamProxyKey)
	if err != nil || row == nil {
		return "", err
	}
	var stored storedUpstreamProxy
	if err := json.Unmarshal([]byte(row.Settings), &stored); err != nil {
		return "", err
	}
	return stored.URL, nil
}

// SaveUpstreamProxy upserts the Upstream Proxy URL; the last write wins.
func (r *Repository) SaveUpstreamProxy(url string, now time.Time) error {
	payload, err := json.Marshal(storedUpstreamProxy{URL: url})
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO common_settings (operation_type, settings_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(operation_type) DO UPDATE SET
			settings_json = excluded.settings_json,
			updated_at = excluded.updated_at
	`, upstreamProxyKey, string(payload), now.Format(timeFormat))
	return err
}
