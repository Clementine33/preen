import { computed, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter, type LocationQuery } from 'vue-router'
import { canonicalRj } from '@/app/route-params'
import type { WorksSort } from '@/lib/api/types'

// The wall's address (ADR 0012 §4): every fact that decides what the wall
// shows — library, filters, FIND, order, page, page size and the selected work
// — lives in the `/works` query and is read from it, never copied at setup. A
// change the page makes to its own address (a nav click, BACK, a link) flows
// straight back in.
//
// Writes go through the explicit actions below, which carry the coordination
// rules: a change to what is listed starts again at page 1, a page turn keeps
// the filters, a library switch drops the previous library's picks. Each
// action resolves once the address has actually changed, so a caller that
// navigates on (FIND, opening a work) acts on the committed address.

export const PAGE_SIZES = [24, 48, 96, 192] as const
export const DEFAULT_SIZE = 48
const SIZE_KEY = 'works.pageSize'
export const DEFAULT_SORT: WorksSort = 'release_date'
/**
 * The longest query the wall commits. A name without a source id is one id per
 * work (ADR 0011 §6), so a pick can carry hundreds of ids; 64 KiB is about
 * 1,700 of them and well inside the backend's 1 MiB header limit. Past it a
 * change is refused out loud rather than dropped into a wider listing.
 */
export const MAX_QUERY = 64 * 1024

type Changes = Record<string, string | undefined>

/** The page-size preference: the starting size of a fresh visit, nothing more. */
function preferredSize(): number {
  try {
    const stored = Number(localStorage.getItem(SIZE_KEY))
    return (PAGE_SIZES as readonly number[]).includes(stored) ? stored : DEFAULT_SIZE
  } catch {
    return DEFAULT_SIZE
  }
}

function savePreferredSize(size: number): void {
  try {
    localStorage.setItem(SIZE_KEY, String(size))
  } catch {
    // A blocked storage only loses the preference; the address keeps the size.
  }
}

export function queryLength(query: LocationQuery | Changes): number {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (typeof value === 'string' && value !== '') params.set(key, value)
  }
  return params.toString().length
}

export function useWallQuery() {
  const route = useRoute()
  const router = useRouter()

  // The last `/works` query: leaving for a work's page must not read as the
  // wall's filters being cleared while the wall is still mounted.
  const current = shallowRef<LocationQuery>(route.query)
  watch(() => (route.name === 'works' ? route.query : null), (query) => {
    if (query) current.value = query
  }, { flush: 'sync' })

  const text = (key: string): string => {
    const value = current.value[key]
    return typeof value === 'string' ? value : ''
  }

  const library = computed(() => text('library') || null)
  const voice = computed(() => text('voice'))
  const circle = computed(() => text('circle'))
  const age = computed(() => text('age'))
  const tag = computed(() => text('tag'))
  const mode = computed<'all' | 'any'>(() => (text('mode') === 'any' ? 'any' : 'all'))
  const sort = computed<WorksSort>(() => (text('sort') as WorksSort) || DEFAULT_SORT)
  const q = computed(() => text('q'))
  const page = computed(() => {
    const parsed = Number(text('page'))
    return Number.isInteger(parsed) && parsed >= 1 ? parsed : 1
  })
  // A named page with no size is read at the fixed default, never a local
  // preference, so the same link lists the same range everywhere.
  const size = computed(() => {
    const parsed = Number(text('size'))
    return (PAGE_SIZES as readonly number[]).includes(parsed) ? parsed : DEFAULT_SIZE
  })
  const rj = computed(() => canonicalRj(text('rj')))
  /** The address itself is past the limit: the wall says so instead of listing. */
  const tooLong = computed(() => queryLength(current.value) > MAX_QUERY)
  /** The last change was refused for its length. */
  const overflow = ref(false)

  // One replace per tick, each built on the address the previous one left:
  // two writes in one tick would otherwise each spread a stale query.
  let pending: Changes | null = null
  let scheduled: Promise<void> = Promise.resolve()
  let chain: Promise<void> = Promise.resolve()
  function commit(changes: Changes): Promise<void> {
    if (pending) {
      Object.assign(pending, changes)
      return scheduled
    }
    pending = { ...changes }
    scheduled = chain = chain.then(async () => {
      const batch = pending ?? {}
      pending = null
      if (route.name !== 'works') return
      const next: Changes = {}
      // Every write names the size in effect, so no later address is left
      // without one for a local preference to reinterpret.
      const sized = route.query.size ? batch : { size: String(size.value), ...batch }
      for (const [key, value] of Object.entries({ ...route.query, ...sized })) {
        if (typeof value === 'string' && value !== '') next[key] = value
      }
      if (queryLength(next) > MAX_QUERY && queryLength(next) > queryLength(route.query)) {
        overflow.value = true
        return
      }
      overflow.value = false
      await router.replace({ query: next })
    }).catch(() => {})
    return scheduled
  }

  // A fresh visit names neither a page nor a size: the preference decides the
  // size once, and the address carries it from then on.
  watch(() => (route.name === 'works' ? route.query : null), (query) => {
    if (query && !query.size && !query.page) void commit({ size: String(preferredSize()) })
  }, { immediate: true })

  return {
    library, voice, circle, age, tag, mode, sort, q, page, size, rj, tooLong, overflow,
    /** Anything that changes what is listed starts again at the first page. */
    setFilters: (changes: Partial<Record<'voice' | 'circle' | 'age' | 'tag' | 'mode' | 'q', string>>) =>
      commit({ ...changes, page: undefined }),
    setSort: (value: WorksSort) => commit({ sort: value === DEFAULT_SORT ? undefined : value, page: undefined }),
    setSize: (value: number) => {
      savePreferredSize(value)
      return commit({ size: String(value), page: undefined })
    },
    /** A page turn keeps every filter. */
    setPage: (value: number) => commit({ page: value > 1 ? String(value) : undefined }),
    /** The user picked another library: its predecessor's picks mean nothing there. */
    setLibrary: (value: string) =>
      commit({ library: value, voice: undefined, circle: undefined, tag: undefined, mode: undefined, page: undefined }),
    /** Names the library without touching anything else (the address had none). */
    fillLibrary: (value: string) => commit({ library: value }),
    select: (value: string | null) => commit({ rj: value ?? undefined }),
  }
}

export type WallQuery = ReturnType<typeof useWallQuery>
