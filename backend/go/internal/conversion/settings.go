package conversion

// PlanConfig selects how a planning pass persists measured bitrates.
type PlanConfig struct {
	// BatchUpdate selects the chunked, transactional write shape for probed
	// bitrates over the per-row one.
	BatchUpdate bool
}

// Settings supplies live planning preferences and draft tag defaults. Missing,
// unreadable or malformed task configuration falls back to defaults. Tool
// dependencies are injected separately by the process assembly.
type Settings interface {
	// Plan reads the planning configuration (plan.bitrate.batch_update).
	Plan() PlanConfig
	// PruneNoEffectTags reads the classifier tag literals a draft seeds from and
	// the tag library offers as its defaults (prune.no_effect_tags).
	PruneNoEffectTags() []string
	// PruneOtherTags seeds independent directory isolation rules.
	PruneOtherTags() []string
}
