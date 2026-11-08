import type { QueryClient } from '@tanstack/vue-query'
import type {
  ApiClientContract,
  CreateLibraryInput,
  FileOperation,
  FileOperationItem,
  FileOperationResult,
  Library,
  MemberTreeResponse,
  UpdateLibraryInput,
} from '@/lib/api/types'
import { useQuery } from '@tanstack/vue-query'
import { computed, watchEffect } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { useLibraryUiStore } from '@/stores/library-ui'
import { refreshOrRemoveQueries } from './cache-sync'
import { usePreferences } from './preferences'
import { TREE_GC_TIME } from './query-client'
import { queryKeys } from './query-keys'
import { sweepWorkCaches } from './works'

// ---------- query options ----------

export function libraryListQueryOptions(api: ApiClientContract) {
  return {
    queryKey: queryKeys.libraries.list(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.listLibraries(signal),
  }
}

/**
 * The overview listing: every direct child directory of the library root,
 * including empty ones and ones holding no audio, with the audio count as a
 * status rather than a filter.
 */
export function dirsQueryOptions(
  api: ApiClientContract,
  libraryId: string | null,
) {
  return {
    queryKey: queryKeys.libraries.dirs(libraryId ?? ''),
    enabled: Boolean(libraryId),
    queryFn: ({ signal }: { signal?: AbortSignal }) => {
      if (!libraryId)
        throw new Error('dirs query requires a library')
      return api.listDirs(libraryId, signal)
    },
  }
}

/**
 * One member tree, addressed by the member's directory identity — the identity
 * the overview lists and the record hands out for the same directory, so both
 * entries read one cache entry. The stored tree is shown while the next refresh
 * lands (gcTime keeps it around after a page leaves), so entering a member page
 * never shows an empty tree first.
 */
export function memberTreeQueryOptions(
  api: ApiClientContract,
  libraryId: string | null,
  dirId: string | null,
) {
  return {
    queryKey: queryKeys.libraries.memberTree(libraryId ?? '', dirId ?? ''),
    enabled: Boolean(libraryId && dirId),
    gcTime: TREE_GC_TIME,
    queryFn: ({ signal }: { signal?: AbortSignal }) => {
      if (!libraryId || !dirId) {
        throw new Error('member tree query requires a library and directory id')
      }
      return api.getMemberTree(libraryId, dirId, signal)
    },
  }
}

/**
 * Refresh one member tree: a scoped scan of that directory. It is refused
 * while a file operation (or a session) holds the admission slot, and the
 * refreshed tree is written straight into the cache the page reads.
 */
export function refreshMemberTreeMutationOptions(
  api: ApiClientContract,
  queryClient: QueryClient,
  libraryId: string,
) {
  return {
    mutationFn: (dirId: string) => api.refreshMemberTree(libraryId, dirId),
    onSuccess: (result: MemberTreeResponse, dirId: string) => {
      queryClient.setQueryData(
        queryKeys.libraries.memberTree(libraryId, dirId),
        result,
      )
    },
  }
}

/**
 * One direct file-management request. The backend answers per item and also
 * reports whether it could refresh the inventory afterwards; both facts reach
 * the caller unchanged. Whatever happened, the caches that describe the disk
 * are refreshed here: the member tree, the overview counts, and the library's
 * current record (whose plan validity the change may have moved).
 */
export function fileOperationMutationOptions(
  api: ApiClientContract,
  queryClient: QueryClient,
  libraryId: string,
) {
  return {
    mutationFn: (input: {
      dirId: string
      body: { member_path: string, operation: FileOperation, items: FileOperationItem[] }
    }) => api.applyFileOperation(libraryId, input.body),
    onSuccess: (result: FileOperationResult, input: { dirId: string }) => {
      void sweepWorkCaches(queryClient)
      void refreshOrRemoveQueries(
        queryClient,
        queryKeys.libraries.memberTree(libraryId, input.dirId),
      )
      void refreshOrRemoveQueries(queryClient, queryKeys.libraries.dirs(libraryId))
      // A rename, move or deletion inside a member changes the inputs a plan
      // was frozen from, so the record's own view is refetched too.
      void refreshOrRemoveQueries(queryClient, queryKeys.worksets.current(libraryId, 'conversion'))
      return result
    },
  }
}

// ---------- library mutations (cache coordination is owned here) ----------

export function createLibraryMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (input: CreateLibraryInput) => api.createLibrary(input),
    onSuccess: (library: Library, _variables: CreateLibraryInput, _context: unknown) => {
      queryClient.setQueryData<Library[]>(queryKeys.libraries.list(), (old) => {
        if (!old)
          return [library]
        if (old.some(item => item.id === library.id))
          return old
        return [...old, library]
      })
    },
  }
}

export function updateLibraryMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: ({ id, input }: { id: string, input: UpdateLibraryInput }) => api.updateLibrary(id, input),
    onSuccess: (
      updated: Library,
      _variables: { id: string, input: UpdateLibraryInput },
      _context: unknown,
    ) => {
      queryClient.setQueryData<Library[]>(queryKeys.libraries.list(), old =>
        old
          ? old.map(item => (item.id === updated.id
              ? updated
              : item))
          : old)
    },
  }
}

export function deleteLibraryMutationOptions(api: ApiClientContract, queryClient: QueryClient) {
  return {
    mutationFn: (id: string) => api.deleteLibrary(id),
    onSuccess: (_result: void, id: string, _context: unknown) => {
      void sweepWorkCaches(queryClient)
      queryClient.setQueryData<Library[]>(queryKeys.libraries.list(), old =>
        old
          ? old.filter(item => item.id !== id)
          : old)
      void refreshOrRemoveQueries(queryClient, queryKeys.libraries.dirs(id))
      void refreshOrRemoveQueries(queryClient, queryKeys.libraries.memberTreesPrefix(id))
      // Deleting a library takes its processing record with it, so every
      // record-scoped cache of that library is dropped rather than refreshed.
      void refreshOrRemoveQueries(queryClient, queryKeys.worksets.all())
    },
  }
}

// ---------- composables ----------

// Observes the library list and the preferences, and keeps the UI store's
// active ID reconciled and its start library mirrored. A page mounts this to
// guarantee data (the app also mounts AppShell as a long-lived observer), so
// any consumer can derive its library list from the shared cache. Exposes the
// derived data so callers do not repeat the same unwrapping/computed logic.
//
// The Default Library counts only while it still exists. `startLibraryId` is
// the library a session with no place of its own opens (the Default Library,
// else the newest); it stays null until the list and the preferences have both
// answered, so a first fill never picks the newest just before the default
// arrives. A failed preferences read counts as answered, with no default.
export function useLibraryList() {
  const api = useApiClient()
  const ui = useLibraryUiStore()
  const query = useQuery(libraryListQueryOptions(api))
  const { query: preferences } = usePreferences()
  const librariesData = computed(() => query.data.value ?? [])
  const defaultLibraryId = computed(() => {
    const id = preferences.data.value?.default_library_id
    return id && librariesData.value.some(library => library.id === id)
      ? id
      : null
  })
  const startLibraryId = computed(() =>
    query.isSuccess.value && !preferences.isPending.value
      ? (defaultLibraryId.value ?? librariesData.value[0]?.id ?? null)
      : null,
  )
  watchEffect(() => {
    if (query.data.value)
      ui.reconcileLibraries(query.data.value)
    ui.startLibraryId = startLibraryId.value
  })
  const activeLibrary = computed(
    () => librariesData.value.find(library => library.id === ui.activeLibraryId) ?? null,
  )
  return {
    query,
    librariesData,
    activeLibrary,
    defaultLibraryId,
    startLibraryId,
  }
}
