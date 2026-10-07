package conversion

// Persisted documents retain read compatibility without weakening HTTP DTOs.
// An explicit empty new list wins over the legacy spelling.
func (d *DraftDoc) foldLegacyTags() {
	if d.NoEffectTags == nil {
		d.NoEffectTags = d.LegacyClassifierTags
	}
	d.LegacyClassifierTags = nil
	for i := range d.Members {
		if o := d.Members[i].Overrides; o != nil {
			o.foldLegacyTags()
		}
	}
}

func (o *OverrideSet) foldLegacyTags() {
	if o.NoEffectTags == nil {
		o.NoEffectTags = o.LegacyClassifierTags
	}
	o.LegacyClassifierTags = nil
}

func (c *CommonSettings) foldLegacyTags() {
	if c.NoEffectTags == nil {
		c.NoEffectTags = c.LegacyClassifierTags
	}
	c.LegacyClassifierTags = nil
}
