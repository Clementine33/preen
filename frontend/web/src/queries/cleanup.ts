import type { CleanupCategory, CleanupRun } from '@/lib/api/types'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import { queryKeys } from './query-keys'

/** How often a live run is re-read; there is no cleanup SSE stream. */
export const CLEANUP_POLL_MS = 2000

export function isCleanupActive(run: CleanupRun | null | undefined): boolean {
  return run?.status === 'running' || run?.status === 'stopping'
}

/**
 * The deployment's current/latest cleanup run (docs/api/maintenance.md). It
 * polls only while a run is live, so leaving and returning shows server state.
 * A failed read keeps the last data: the state is then unknown, not changed.
 */
export function useCleanupRun() {
  const api = useApiClient()
  return useQuery({
    queryKey: queryKeys.cleanup.run(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getCleanup(signal),
    staleTime: 0,
    refetchOnMount: 'always',
    refetchInterval: query => (isCleanupActive(query.state.data)
      ? CLEANUP_POLL_MS
      : false),
  })
}

export function usePreviewCleanup() {
  const api = useApiClient()
  return useMutation({
    mutationFn: (categories: CleanupCategory[]) => api.previewCleanup(categories),
  })
}

/** Starts once per confirmation; a lost answer is resolved by reading the run, never by resending. */
export function useStartCleanup() {
  const api = useApiClient()
  const client = useQueryClient()
  return useMutation({
    mutationFn: (categories: CleanupCategory[]) => api.startCleanup(categories),
    onSuccess: run => client.setQueryData(queryKeys.cleanup.run(), run),
  })
}

export function useCancelCleanup() {
  const api = useApiClient()
  const client = useQueryClient()
  return useMutation({
    mutationFn: (runId: string) => api.cancelCleanup(runId),
    onSuccess: run => client.setQueryData(queryKeys.cleanup.run(), run),
  })
}
