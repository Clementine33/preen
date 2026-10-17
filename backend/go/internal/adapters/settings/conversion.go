package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/onsei/organizer/backend/internal/conversion"
)

// Reader implements conversion.Settings over config.json in one directory.
// Reads use live task preferences with defaults on failures; worker limits and
// media tool paths are not file configuration.
type Reader struct{ configDir string }

// NewReader roots a reader at one config directory.
func NewReader(configDir string) Reader { return Reader{configDir: configDir} }

// Plan reads the planning key plan.bitrate.batch_update.
// A missing file is not a failure — the compiled-in defaults are the answer —
// and neither is one that cannot be read or parsed: a plan is never failed by
// its configuration, and it never runs on a half-read one either.
func (r Reader) Plan() conversion.PlanConfig {
	defaults := DefaultAppConfig()
	out := conversion.PlanConfig{BatchUpdate: defaults.Plan.Bitrate.BatchUpdate}

	// Decoded over the defaults, never over a zero value: a key the file omits
	// keeps its default, so a configuration that names only unrelated keys
	// still writes probed rates in batches. A file that is missing, unreadable
	// or unparsable leaves the defaults alone.
	cfg := DefaultAppConfig()
	if !r.decode(&cfg) {
		return out
	}
	out.BatchUpdate = cfg.Plan.Bitrate.BatchUpdate
	return out
}

// PruneNoEffectTags reads the maintained initial literal tag list
// (prune.no_effect_tags): what a new draft seeds from, and what the tag library
// offers as its defaults. A missing/unreadable file yields an empty set — there
// is deliberately no compiled-in fallback.
func (r Reader) PruneNoEffectTags() []string {
	cfg := DefaultAppConfig()
	if !r.decode(&cfg) {
		return nil
	}
	return normalizeTags(cfg.Prune.NoEffectTags)
}

// decode loads config.json into cfg, reporting whether there was a readable one
// to load. "There is no configuration" and "there is a broken one" are the same
// answer here: every read above treats the file as optional.
func (r Reader) decode(cfg *AppConfig) bool {
	if r.configDir == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(r.configDir, "config.json"))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, cfg) == nil
}

// PruneOtherTags reads the maintained directory isolation defaults.
func (r Reader) PruneOtherTags() []string {
	cfg := DefaultAppConfig()
	if !r.decode(&cfg) {
		return nil
	}
	return normalizeTags(cfg.Prune.OtherTags)
}
