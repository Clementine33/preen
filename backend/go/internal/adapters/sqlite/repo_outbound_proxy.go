package sqlite

import (
	"encoding/json"
	"time"

	"github.com/Clementine33/preen/backend/internal/works"
)

// outboundProxyKey is the common_settings row holding the Outbound Proxy.
// Like preferences it is not a Task kind.
const outboundProxyKey = "outbound_proxy"

// OutboundProxy returns the stored Outbound Proxy, system mode when never
// saved. A row written before modes existed reads as manual when it holds a
// URL and direct otherwise. A row that cannot be decoded is an error: callers
// must not treat it as direct.
func (r *Repository) OutboundProxy() (works.OutboundProxy, error) {
	row, err := r.GetCommonSettings(outboundProxyKey)
	if err != nil || row == nil {
		return works.OutboundProxy{Mode: works.ProxySystem}, err
	}
	var stored works.OutboundProxy
	if err := json.Unmarshal([]byte(row.Settings), &stored); err != nil {
		return works.OutboundProxy{}, err
	}
	if stored.Mode == "" {
		stored.Mode = works.ProxyDirect
		if stored.URL != "" {
			stored.Mode = works.ProxyManual
		}
	}
	return stored, nil
}

// SaveOutboundProxy upserts the Outbound Proxy; the last write wins.
func (r *Repository) SaveOutboundProxy(proxy works.OutboundProxy, now time.Time) error {
	payload, err := json.Marshal(proxy)
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
