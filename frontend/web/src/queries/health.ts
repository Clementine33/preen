import type { ApiClientContract } from '@/lib/api/types'
import { queryKeys } from './query-keys'

/** Server build and selection limits, shared by startup, settings and the overview. */
export function healthQueryOptions(api: ApiClientContract) {
  return {
    queryKey: queryKeys.health(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getHealth(signal),
    retry: false,
  }
}
