import type { MaybeRefOrGetter } from 'vue'
import type { LocalCover } from '@/lib/api/types'
import { computed, inject, ref, toValue } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import { coverEpoch } from '@/queries/cover-epoch'

interface CoverSource {
  rj: string
  local?: LocalCover | null
  /** The scraped remote link: the fallback while the local file cannot be read. */
  remote: string | null
}

/**
 * The address a work's cover is shown from. A cover already on disk is served
 * by the backend's content route; it is shown ahead of the remote link until
 * it fails to load, and a failure holds only until the next list/detail read
 * lands (see coverEpoch) — a version change reloads on its own, as it changes
 * the address.
 */
export function useCoverSrc(source: MaybeRefOrGetter<CoverSource | null>) {
  const api = inject(apiClientKey, null)
  const failures = ref(new Map<string, number>())
  const local = computed(() => {
    const s = toValue(source)
    if (!s?.local || !api)
      return null
    const { copy_id: copyId, rel_path: relPath, version } = s.local
    return api.workFileUrl({ rj: s.rj, copyId, relPath }, version)
  })
  const src = computed(() => {
    const url = local.value
    if (url !== null && failures.value.get(url) !== coverEpoch.value)
      return url
    return toValue(source)?.remote ?? null
  })
  /** Report that `url` did not load; only the local address is tracked. */
  function failed(url: string): void {
    if (url === local.value)
      failures.value = new Map(failures.value).set(url, coverEpoch.value)
  }
  return {
    src,
    failed,
    isLocal: (url: string | null) => url !== null && url === local.value,
  }
}
