import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import { queryKeys } from './query-keys'

/** The server-stored Upstream Proxy (docs/api/upstream-proxy.md). */
export function useUpstreamProxy() {
  const api = useApiClient()
  return useQuery({
    queryKey: queryKeys.upstreamProxy.detail(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getUpstreamProxy(signal),
  })
}

/** Saves the URL; the cache takes the server's trimmed answer. */
export function useSetUpstreamProxy() {
  const api = useApiClient()
  const client = useQueryClient()
  return useMutation({
    mutationFn: (url: string) => api.putUpstreamProxy({ url }),
    onSuccess: (saved) => client.setQueryData(queryKeys.upstreamProxy.detail(), saved),
  })
}
