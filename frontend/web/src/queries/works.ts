import { useInfiniteQuery, useQuery } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useApiClient } from '@/lib/api/client'
import type { ApiClientContract, ListWorksParams, WorkListResponse } from '@/lib/api/types'
import { queryKeys } from './query-keys'

// ---------- query options ----------

/**
 * The works listing, one keyset page at a time. The cursor is opaque and only
 * valid for the exact filter fingerprint it was issued under, so the options
 * carry the whole filter set in one object that never changes mid-page.
 */
export function worksListQueryOptions(api: ApiClientContract, filters: ListWorksParams) {
  return {
    queryKey: queryKeys.works.list(filters),
    queryFn: ({ signal, pageParam }: { signal?: AbortSignal; pageParam?: string }) =>
      api.listWorks({ ...filters, cursor: pageParam }, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage: WorkListResponse) => lastPage.next_cursor,
  }
}

/**
 * One work in full. The rj may be null while nothing is selected: the query
 * sits disabled instead of firing with an empty work number.
 */
export function workDetailQueryOptions(api: ApiClientContract, rj: string | null) {
  return {
    queryKey: queryKeys.works.detail(rj ?? ''),
    enabled: rj !== null,
    queryFn: ({ signal }: { signal?: AbortSignal }) => {
      if (rj === null) throw new Error('work detail query requires a work number')
      return api.getWork(rj, signal)
    },
  }
}

// ---------- composables ----------

/**
 * The works listing behind a reactive filter set (the active library, the
 * filter tab, the FIND box). Pages read `works` as one flat list; pages and
 * cursors accumulate while the filter identity stays unchanged.
 */
export function useWorksList(filters: MaybeRefOrGetter<ListWorksParams>) {
  const api = useApiClient()
  const query = useInfiniteQuery(computed(() => worksListQueryOptions(api, toValue(filters))))
  const works = computed(() => query.data.value?.pages.flatMap((page) => page.works) ?? [])
  return { query, works }
}

/** The full work behind a reactive work number, disabled while none is set. */
export function useWorkDetail(rj: MaybeRefOrGetter<string | null>) {
  const api = useApiClient()
  const query = useQuery(computed(() => workDetailQueryOptions(api, toValue(rj))))
  const work = computed(() => query.data.value ?? null)
  return { query, work }
}
