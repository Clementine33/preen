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
  /** Load a local cover from the thumbnails route at this height instead of the file itself. */
  thumbHeight?: number
}

/**
 * The CDN's small rendition of a main cover: a presentation of that cover, so
 * the display that wants it derives it. Other addresses stay as they are.
 */
export function remoteThumb(cover: string | null | undefined): string | null {
  if (!cover)
    return null
  let url: URL
  try {
    url = new URL(cover)
  }
  catch {
    return cover
  }
  const { host, pathname } = url
  if (host === 'img.dlsite.jp' && pathname.startsWith('/modpub/') && /\.[^./]*$/.test(pathname)) {
    url.pathname = pathname.replace('/modpub/', '/resize/').replace(/\.[^./]*$/, '_240x240.jpg')
    return url.href
  }
  if (host === 'api.asmr.one' && pathname.startsWith('/api/cover/')) {
    url.searchParams.set('type', '240x240')
    return url.href
  }
  return cover
}

/**
 * The address a work's cover is shown from. A cover already on disk is served
 * by the backend's content route; it is shown ahead of the remote link until
 * it fails to load, and the remote link is shown until it fails too (then
 * null). A failure holds only until the next list/detail read lands (see
 * coverEpoch) — a version change reloads on its own, as it changes the
 * address.
 */
export function useCoverSrc(source: MaybeRefOrGetter<CoverSource | null>) {
  const api = inject(apiClientKey, null)
  const failures = ref(new Map<string, number>())
  const local = computed(() => {
    const s = toValue(source)
    if (!s?.local || !api)
      return null
    const { copy_id: copyId, rel_path: relPath, version, thumb_v: thumbV } = s.local
    return s.thumbHeight
      ? api.workThumbUrl({ rj: s.rj, copyId, relPath }, s.thumbHeight, thumbV)
      : api.workFileUrl({ rj: s.rj, copyId, relPath }, version)
  })
  const src = computed(() => {
    const url = local.value
    if (url !== null && failures.value.get(url) !== coverEpoch.value)
      return url
    const remote = toValue(source)?.remote ?? null
    return remote !== null && failures.value.get(remote) !== coverEpoch.value
      ? remote
      : null
  })
  /** Report that `url` (the local or the remote address) did not load. */
  function failed(url: string): void {
    if (url === local.value || url === toValue(source)?.remote)
      failures.value = new Map(failures.value).set(url, coverEpoch.value)
  }
  return { src, failed }
}
