import type { DeleteMode, DesiredProfile, OverrideUnit } from '@/lib/api/types'

/**
 * The values a new conversion draft is seeded with (backend `seedDraft`):
 * relaxed mode, the deployment's literal tags, MP3@320 without lossless output for all
 * version targets, and soft deletion of obsolete audio. 恢复默认 restores exactly
 * these, so "default" means one thing.
 */
export function defaultDraftValues(
  defaultTags: string[],
  defaultOtherTags: string[] = [],
): Record<OverrideUnit, unknown> & { delete_mode: DeleteMode } {
  const profile: DesiredProfile = {
    encoded: { codec: 'mp3', quality: { kind: 'bitrate', bitrate: 320 } },
  }
  return {
    mode: 'available_sources',
    no_effect_tags: [...defaultTags],
    other_tags: [...defaultOtherTags],
    matched: profile,
    unmatched: profile,
    other: profile,
    delete_mode: 'soft',
  }
}
