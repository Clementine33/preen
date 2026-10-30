import { QueryClient } from '@tanstack/vue-query'

// Cached data defaults to explicit mutation/scan synchronization. Resources
// with live validation override freshness and refetch-on-mount in their queries.
export const REFETCH_ON_MOUNT = false
export const REFETCH_ON_WINDOW_FOCUS = false
export const REFETCH_ON_RECONNECT = false

// Inactive caches are retained for a bounded window; large trees use a
// shorter TTL via per-query override so a long session cannot grow unbounded.
export const DEFAULT_GC_TIME = 30 * 60 * 1000
export const TREE_GC_TIME = 10 * 60 * 1000

export function createAppQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: Infinity,
        gcTime: DEFAULT_GC_TIME,
        retry: false,
        refetchOnMount: REFETCH_ON_MOUNT,
        refetchOnWindowFocus: REFETCH_ON_WINDOW_FOCUS,
        refetchOnReconnect: REFETCH_ON_RECONNECT,
      },
      mutations: {
        retry: false,
      },
    },
  })
}
