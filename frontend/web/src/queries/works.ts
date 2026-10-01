import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import { useApiClient } from '@/lib/api/client'
import type {
  ApiClientContract,
  ListWorksParams,
  WorkFacetKind,
  WorkListResponse,
  WorkUserTag,
} from '@/lib/api/types'
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
    // A filter change keeps the previous wall on screen until the new listing
    // lands, instead of blanking it; `isPlaceholderData` says which one shows.
    placeholderData: keepPreviousData,
  }
}

/** The circles or voice actors listed works carry, under one library scope. */
export function workFacetsQueryOptions(api: ApiClientContract, kind: WorkFacetKind, library: string | null) {
  return {
    queryKey: queryKeys.works.facets(kind, library ?? ''),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listWorkFacets(kind, library ?? undefined, signal),
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

/** One facet list behind a reactive library scope (null = every library). */
export function useWorkFacets(kind: WorkFacetKind, library: MaybeRefOrGetter<string | null>) {
  const api = useApiClient()
  const query = useQuery(computed(() => workFacetsQueryOptions(api, kind, toValue(library))))
  const facets = computed(() => query.data.value ?? [])
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
  const merge = mergeIntoWorkCache(() => toValue(rj))
  return useMutation({
    mutationFn: (tagIds: string[]) => {
      const value = toValue(rj)
      if (value === null) throw new Error('a user-tag write requires a work number')
      return api.replaceWorkUserTags(value, tagIds)
    },
    onSuccess: (userTags) => merge({ user_tags: userTags }),
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
