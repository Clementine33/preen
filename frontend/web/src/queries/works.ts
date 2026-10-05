import { keepPreviousData, useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useApiClient } from '@/lib/api/client'
import type {
  ApiClientContract,
  ListWorksParams,
  WorkFacetKind,
  WorkUserTag,
  WorksScope,
} from '@/lib/api/types'
import { queryKeys } from './query-keys'
import { refreshOrRemoveQueries } from './cache-sync'

/** Inventory and bindings changed; personal tag vocabulary is unaffected. */
export async function sweepWorkCaches(client: QueryClient): Promise<void> {
  await Promise.all([
    queryKeys.works.listPrefix(), queryKeys.works.detailPrefix(),
    queryKeys.works.contentPrefix(), queryKeys.works.facetsPrefix(),
  ].map((prefix) => refreshOrRemoveQueries(client, prefix)))
}

// ---------- query options ----------

/** One numbered page of the works listing, with the listing's total. */
export function worksListQueryOptions(api: ApiClientContract, filters: ListWorksParams) {
  return {
    queryKey: queryKeys.works.list(filters),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listWorks(filters, signal),
  }
}

/**
 * The circles or voice actors the works of one listing scope carry. A scope
 * change keeps the previous options open instead of emptying the rail.
 */
export function workFacetsQueryOptions(api: ApiClientContract, kind: WorkFacetKind, scope: WorksScope) {
  return {
    queryKey: queryKeys.works.facets(kind, scope),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listWorkFacets(kind, scope, signal),
    placeholderData: keepPreviousData,
  }
}

/** Both tag vocabularies under one listing scope, the tag index's two sections. */
export function tagFacetsQueryOptions(api: ApiClientContract, scope: WorksScope) {
  return {
    queryKey: queryKeys.works.tagFacets(scope),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listTagFacets(scope, signal),
    placeholderData: keepPreviousData,
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
 * One page of the works listing behind a reactive filter set. A filter or page
 * change keeps the previous page on screen until the new one lands instead of
 * blanking the wall; `isPlaceholderData` says which one shows, and nothing may
 * correct the page or the selection from a placeholder.
 */
export function useWorksList(filters: MaybeRefOrGetter<ListWorksParams>, enabled: MaybeRefOrGetter<boolean> = true) {
  const api = useApiClient()
  const query = useQuery(computed(() => ({
    ...worksListQueryOptions(api, toValue(filters)),
    placeholderData: keepPreviousData,
    enabled: toValue(enabled),
  })))
  const works = computed(() => query.data.value?.works ?? [])
  const total = computed(() => query.data.value?.total ?? 0)
  return { query, works, total }
}

/**
 * One facet list behind a reactive listing scope. `enabled` lets a caller ask
 * only when it needs the answer (a name lookup for a pick the narrowed list no
 * longer carries).
 */
export function useWorkFacets(
  kind: WorkFacetKind,
  scope: MaybeRefOrGetter<WorksScope>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const api = useApiClient()
  const query = useQuery(computed(() => ({
    ...workFacetsQueryOptions(api, kind, toValue(scope)),
    enabled: toValue(enabled),
  })))
  const facets = computed(() => query.data.value ?? [])
  return { query, facets }
}

/** The tag index's entries behind a reactive listing scope; `enabled` as above. */
export function useTagFacets(scope: MaybeRefOrGetter<WorksScope>, enabled: MaybeRefOrGetter<boolean> = true) {
  const api = useApiClient()
  const query = useQuery(computed(() => ({
    ...tagFacetsQueryOptions(api, toValue(scope)),
    enabled: toValue(enabled),
  })))
  const facets = computed(() => query.data.value ?? { source_tags: [], user_tags: [] })
  return { query, facets }
}

/** The full work behind a reactive work number, disabled while none is set. */
export function useWorkDetail(rj: MaybeRefOrGetter<string | null>) {
  const api = useApiClient()
  const query = useQuery(computed(() => workDetailQueryOptions(api, toValue(rj))))
  const work = computed(() => query.data.value ?? null)
  return { query, work }
}

/** One work's file manifest (docs/api.md "Work content manifest"). */
export function useWorkContent(rj: MaybeRefOrGetter<string | null>) {
  const api = useApiClient()
  const query = useQuery(
    computed(() => ({
      queryKey: queryKeys.works.content(toValue(rj) ?? ''),
      enabled: toValue(rj) !== null,
      queryFn: ({ signal }: { signal?: AbortSignal }) => {
        const value = toValue(rj)
        if (value === null) throw new Error('work content query requires a work number')
        return api.getWorkContent(value, signal)
      },
    })),
  )
  const content = computed(() => query.data.value ?? null)
  return { query, content }
}

/** The account's own tag vocabulary (docs/api.md `GET /user-tags`). */
export function useUserTags() {
  const api = useApiClient()
  const query = useQuery({
    queryKey: queryKeys.works.userTags(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listUserTags(signal),
  })
  const tags = computed<WorkUserTag[]>(() => query.data.value ?? [])
  return { query, tags }
}

/** Merges a personal-data patch into the cached work, keeping every other field. */
function mergeIntoWorkCache(rj: () => string | null) {
  const client = useQueryClient()
  return (patch: object) => {
    const key = rj()
    if (key === null) return
    client.setQueryData<Record<string, unknown>>(queryKeys.works.detail(key), (work) =>
      work ? { ...work, ...patch } : work,
    )
  }
}

/** The work's personal data, written through and merged into the cached detail. */
export function useUpdateWorkUserData(rj: MaybeRefOrGetter<string | null>) {
  const api = useApiClient()
  const merge = mergeIntoWorkCache(() => toValue(rj))
  return useMutation({
    mutationFn: (input: Parameters<ApiClientContract['updateWorkUserData']>[1]) => {
      const value = toValue(rj)
      if (value === null) throw new Error('a personal-data write requires a work number')
      return api.updateWorkUserData(value, input)
    },
    onSuccess: merge,
  })
}

/** Replaces the work's user-tag set and merges the answer into the cached detail. */
export function useReplaceWorkUserTags(rj: MaybeRefOrGetter<string | null>) {
  const api = useApiClient()
  const client = useQueryClient()
  const merge = mergeIntoWorkCache(() => toValue(rj))
  return useMutation({
    mutationFn: (tagIds: string[]) => {
      const value = toValue(rj)
      if (value === null) throw new Error('a user-tag write requires a work number')
      return api.replaceWorkUserTags(value, tagIds)
    },
    onSuccess: (userTags) => {
      merge({ user_tags: userTags })
      // The tag index counts this work under its tags, and a listing filtered
      // by a user tag may now gain or lose it.
      void refreshOrRemoveQueries(client, queryKeys.works.tagFacetsPrefix())
      void refreshOrRemoveQueries(client, queryKeys.works.listPrefix())
    },
  })
}

/** Creates a tag by name; the route is idempotent, an existing name answers it. */
export function useCreateUserTag() {
  const api = useApiClient()
  const client = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.createUserTag(name),
    onSuccess: (tag) => {
      client.setQueryData<WorkUserTag[]>(queryKeys.works.userTags(), (tags) =>
        tags ? [...tags.filter((entry) => entry.id !== tag.id), tag] : [tag],
      )
    },
  })
}
