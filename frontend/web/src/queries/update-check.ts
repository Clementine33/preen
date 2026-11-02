import type { Query } from '@tanstack/vue-query'
import type { ApiClientContract, UpdateCheck } from '@/lib/api/types'
import { queryKeys } from './query-keys'

const CHECKING_POLL_MS = 5000

type UpdateCheckQuery = Query<UpdateCheck, Error, UpdateCheck, ReturnType<typeof queryKeys.updateCheck>>

/**
 * The server's one release check (docs/api/update-check.md). It is polled
 * while the server is still `checking` and settles for the process lifetime,
 * so the toast composable and the About page share one cache entry.
 */
export function updateCheckQueryOptions(api: ApiClientContract) {
  return {
    queryKey: queryKeys.updateCheck(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getUpdateCheck(signal),
    retry: false,
    refetchInterval: (query: UpdateCheckQuery) => query.state.data?.state === 'checking'
      ? CHECKING_POLL_MS
      : false,
  }
}
