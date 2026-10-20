import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import { queryKeys } from './query-keys'

/** The server-stored Outbound Proxy (docs/api/outbound-proxy.md). */
export function useOutboundProxy() {
  const api = useApiClient()
  return useQuery({
    queryKey: queryKeys.outboundProxy.detail(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getOutboundProxy(signal),
  })
}

/** Saves the URL; the cache takes the server's trimmed answer. */
export function useSetOutboundProxy() {
  const api = useApiClient()
  const client = useQueryClient()
  return useMutation({
    mutationFn: (url: string) => api.putOutboundProxy({ url }),
    onSuccess: (saved) => client.setQueryData(queryKeys.outboundProxy.detail(), saved),
  })
}
