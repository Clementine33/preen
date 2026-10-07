package reconcile

// FoldLegacyTags normalizes a persisted policy before planning. HTTP writes
// reject the legacy key instead of calling this compatibility operation.
func (p *Policy) FoldLegacyTags() {
	if p.NoEffectTags == nil {
		p.NoEffectTags = p.LegacyClassifierTags
	}
	p.LegacyClassifierTags = nil
}
