import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useApiClient } from '@/lib/api/client'
import type { ApiClientContract, CollectionWindow, WorksSort } from '@/lib/api/types'
import { queryKeys } from './query-keys'

/** Every collection, Favorites first; one page holds up to the API's 200. */
export function collectionsQueryOptions(api: ApiClientContract) {
  return {
    queryKey: queryKeys.collections.list(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listCollections({ limit: 200 }, signal),
  }
}

/** One window of a collection's members; null id leaves the query disabled. */
export function collectionDetailQueryOptions(api: ApiClientContract, id: string | null, window: CollectionWindow) {
  return {
    queryKey: queryKeys.collections.detail(id ?? '', window),
    enabled: id !== null,
    queryFn: ({ signal }: { signal?: AbortSignal }) => {
      if (id === null) throw new Error('collection detail query requires an id')
      return api.getCollection(id, window, signal)
    },
  }
}

export function useCollections() {
  const api = useApiClient()
  const query = useQuery(collectionsQueryOptions(api))
  const collections = computed(() => query.data.value?.collections ?? [])
  return { query, collections }
}

/**
 * The newest titles of a collection (first window, newest first). The previous
 * collection's titles stay up while the next one loads, as the wall keeps its page.
 * A null limit (the column not measured yet) holds the query back.
 */
export function useCollectionTitles(id: MaybeRefOrGetter<string | null>, limit: MaybeRefOrGetter<number | null>) {
  const api = useApiClient()
  const query = useQuery(computed(() => ({
    ...collectionDetailQueryOptions(api, toValue(limit) === null ? null : toValue(id), { limit: toValue(limit) ?? 1, direction: 'desc' }),
    placeholderData: keepPreviousData,
  })))
  return { query, titles: computed(() => query.data.value?.works ?? []) }
}

/** Members per chunk of the detail page's list: the API's page maximum. */
export const COLLECTION_CHUNK = 200

/**
 * One collection's members as a list that grows chunk by chunk, in the saved
 * order (`sort` empty) or a view sort. A new sort is a new list.
 */
export function useCollectionPages(id: MaybeRefOrGetter<string>, sort: MaybeRefOrGetter<WorksSort | ''>, direction: MaybeRefOrGetter<'asc' | 'desc'>) {
  const api = useApiClient()
  const query = useInfiniteQuery(computed(() => {
    const [cid, s, dir] = [toValue(id), toValue(sort), toValue(direction)]
    return {
      queryKey: queryKeys.collections.pages(cid, s, dir),
      initialPageParam: 1,
      queryFn: ({ pageParam, signal }: { pageParam: number; signal?: AbortSignal }) =>
        api.getCollection(cid, { limit: COLLECTION_CHUNK, page: pageParam, direction: dir, ...(s ? { sort: s } : {}) }, signal),
      getNextPageParam: (last: { total: number }, all: unknown[]) => (all.length * COLLECTION_CHUNK < last.total ? all.length + 1 : undefined),
    }
  }))
  const first = computed(() => query.data.value?.pages[0] ?? null)
  const works = computed(() => query.data.value?.pages.flatMap((p) => p.works) ?? [])
  return { query, name: computed(() => first.value?.name ?? ''), total: computed(() => first.value?.total ?? 0), works }
}
