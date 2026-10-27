import type { QueryClient } from '@tanstack/vue-query'
import type { MaybeRefOrGetter } from 'vue'
import type { ApiClientContract, CollectionWindow, WorksSort } from '@/lib/api/types'
import { keepPreviousData, useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, toValue } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { refreshOrRemoveQueries } from './cache-sync'
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
      if (id === null)
        throw new Error('collection detail query requires an id')
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
    ...collectionDetailQueryOptions(api, toValue(limit) === null
      ? null
      : toValue(id), { limit: toValue(limit) ?? 1, direction: 'desc' }),
    placeholderData: keepPreviousData,
  })))
  return {
    query,
    titles: computed(() => query.data.value?.works ?? []),
  }
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
    const [cid, s, dir] = [
      toValue(id),
      toValue(sort),
      toValue(direction),
    ]
    return {
      queryKey: queryKeys.collections.pages(cid, s, dir),
      initialPageParam: 1,
      queryFn: ({ pageParam, signal }: { pageParam: number, signal?: AbortSignal }) =>
        api.getCollection(cid, {
          limit: COLLECTION_CHUNK,
          page: pageParam,
          direction: dir,
          ...(s
            ? { sort: s }
            : {}),
        }, signal),
      getNextPageParam: (last: { total: number }, all: unknown[]) => (all.length * COLLECTION_CHUNK < last.total
        ? all.length + 1
        : undefined),
    }
  }))
  const first = computed(() => query.data.value?.pages[0] ?? null)
  const works = computed(() => query.data.value?.pages.flatMap(p => p.works) ?? [])
  return {
    query,
    name: computed(() => first.value?.name ?? ''),
    total: computed(() => first.value?.total ?? 0),
    works,
  }
}

/**
 * Every collection write refreshes the whole `['collections']` prefix (entry
 * counts, titles, detail chunks) and each touched work's detail (`favorite` and
 * `collection_ids` follow membership); a write to Favorites also refreshes the work listings.
 */
function refreshAfterWrite(client: QueryClient, id: string, rjs: string[]): void {
  void refreshOrRemoveQueries(client, queryKeys.collections.all())
  if (id === 'favorites')
    void refreshOrRemoveQueries(client, queryKeys.works.listPrefix())
  for (const rj of rjs) void refreshOrRemoveQueries(client, queryKeys.works.detail(rj))
}

/** Member writes. Each refreshes the caches whether it succeeded or not, so a failure reloads the truth. */
export function useCollectionMemberWrites() {
  const api = useApiClient()
  const client = useQueryClient()
  async function write<T>(id: string, rjs: string[], run: () => Promise<T>): Promise<T> {
    try { return await run() }
    finally { refreshAfterWrite(client, id, rjs) }
  }
  return {
    append: (id: string, rjs: string[]) => write(id, rjs, () => api.appendToCollection(id, rjs)),
    remove: (id: string, rj: string) => write(id, [rj], () => api.removeFromCollection(id, rj)),
    move: (id: string, rj: string, beforeRj: string | null) => write(id, [rj], () => api.moveCollectionMember(id, rj, beforeRj)),
  }
}

/** The default name of the next collection: 新合集 #N, N counting the user collections (not Favorites) plus one. */
export function newCollectionName(collections: { id: string }[]): string {
  return `新合集 #${collections.filter(c => c.id !== 'favorites').length + 1}`
}

/** Create, rename and delete refresh the `['collections']` prefix; a deleted collection's queries are dropped instead, so nothing refetches into a 404. */
export function useCollectionManage() {
  const api = useApiClient()
  const client = useQueryClient()
  return {
    create: async (name: string) => {
      try { return await api.createCollection(name) }
      finally { void refreshOrRemoveQueries(client, queryKeys.collections.all()) }
    },
    rename: async (id: string, name: string) => {
      try { await api.renameCollection(id, name) }
      finally { void refreshOrRemoveQueries(client, queryKeys.collections.all()) }
    },
    remove: async (id: string) => {
      await api.deleteCollection(id)
      client.removeQueries({
        queryKey: [
          ...queryKeys.collections.all(),
          'pages',
          id,
        ],
      })
      client.removeQueries({
        queryKey: [
          ...queryKeys.collections.all(),
          'detail',
          id,
        ],
      })
      void refreshOrRemoveQueries(client, queryKeys.collections.all())
    },
  }
}
