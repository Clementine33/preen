package sqlite

import (
	"encoding/json"
	"time"
)

// outboundProxyKey is the common_settings row holding the Outbound Proxy URL.
// Like preferences it is not a Task kind.
const outboundProxyKey = "outbound_proxy"

type storedOutboundProxy struct {
	URL string `json:"url"`
}

// OutboundProxy returns the stored Outbound Proxy URL, "" when never saved. A
// row that cannot be decoded is an error: callers must not treat it as direct.
func (r *Repository) OutboundProxy() (string, error) {
	row, err := r.GetCommonSettings(outboundProxyKey)
	if err != nil || row == nil {
		return "", err
	}
	var stored storedOutboundProxy
	if err := json.Unmarshal([]byte(row.Settings), &stored); err != nil {
		return "", err
	}
	return stored.URL, nil
}

// SaveOutboundProxy upserts the Outbound Proxy URL; the last write wins.
func (r *Repository) SaveOutboundProxy(url string, now time.Time) error {
	payload, err := json.Marshal(storedOutboundProxy{URL: url})
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO common_settings (operation_type, settings_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(operation_type) DO UPDATE SET
			settings_json = excluded.settings_json,
			updated_at = excluded.updated_at
	`, outboundProxyKey, string(payload), now.Format(timeFormat))
	return err
}
