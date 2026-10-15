import { keepPreviousData, useQuery } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useApiClient } from '@/lib/api/client'
import type { ApiClientContract, CollectionWindow } from '@/lib/api/types'
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
