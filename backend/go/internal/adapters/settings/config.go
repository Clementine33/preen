package settings

import (
	"sort"
	"strings"
)

// PruneConfig defines prune-related settings in config.json. NoEffectTags is
// the maintained initial classifier tag list (matched as case-insensitive
// literal substrings); it seeds new workset drafts only.
type PruneConfig struct {
	NoEffectTags []string `json:"no_effect_tags"`
	OtherTags    []string `json:"other_tags"`
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]string, len(tags))
	for _, raw := range tags {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; !ok {
			seen[key] = trimmed
		}
	}
	out := make([]string, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

type PlanConfig struct {
	Slim        PlanSlimConfig        `json:"slim"`
	RootResolve PlanRootResolveConfig `json:"root_resolve"`
	Bitrate     PlanBitrateConfig     `json:"bitrate"`
}

type PlanSlimConfig struct {
	RequireScope bool `json:"require_scope"`
}

type PlanRootResolveConfig struct {
	Batch bool `json:"batch"`
}

type PlanBitrateConfig struct {
	BatchUpdate bool `json:"batch_update"`
}

// AppConfig represents the full application configuration from config.json.
type AppConfig struct {
	Server ServerConfig `json:"server"`
	Prune  PruneConfig  `json:"prune"`
	Plan   PlanConfig   `json:"plan"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		Plan: PlanConfig{
			Slim: PlanSlimConfig{
				RequireScope: true,
			},
			RootResolve: PlanRootResolveConfig{
				Batch: true,
			},
			Bitrate: PlanBitrateConfig{
				BatchUpdate: true,
			},
		},
	}
}
