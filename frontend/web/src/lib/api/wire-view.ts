/** Wire-to-view conversion preserves server bytes and normalizes legacy editor settings. */
/**
 * Stored conversion JSON may still use the old key; writes only use the new one.
 * Overrides preserve absence, while common settings materialize empty arrays.
 */
export function normalizeConversionTags<T extends { no_effect_tags?: string[] | null, other_tags?: string[] | null }>(value: T, override = false): T {
  const out = { ...value } as T & { classifier_tags?: string[] }
  if (!override || out.no_effect_tags != null || out.classifier_tags != null) {
    out.no_effect_tags = out.no_effect_tags ?? out.classifier_tags ?? []
  }
  else {
    delete out.no_effect_tags
  }
  if (!override)
    out.other_tags = out.other_tags ?? []
  else if (out.other_tags == null)
    delete out.other_tags
  delete out.classifier_tags
  return out
}
