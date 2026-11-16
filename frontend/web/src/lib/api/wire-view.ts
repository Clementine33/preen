/** Wire-to-view conversion preserves server bytes and normalizes stored editor settings. */
/**
 * Stored conversion JSON may carry null or absent tag lists. Overrides preserve
 * absence, while common settings materialize empty arrays.
 */
export function normalizeConversionTags<T extends { no_effect_tags?: string[] | null, other_tags?: string[] | null }>(value: T, override = false): T {
  const out = { ...value }
  if (!override) {
    out.no_effect_tags = out.no_effect_tags ?? []
    out.other_tags = out.other_tags ?? []
  }
  else {
    if (out.no_effect_tags == null)
      delete out.no_effect_tags
    if (out.other_tags == null)
      delete out.other_tags
  }
  return out
}
