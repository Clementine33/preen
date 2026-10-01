import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { onBeforeUnmount, onMounted } from 'vue'
import { ApiError, backOff, useApiClient } from '@/lib/api/client'
import type { ApiClientContract } from '@/lib/api/types'
import { refreshOrRemoveQueries } from '@/queries/cache-sync'
import { queryKeys } from '@/queries/query-keys'

/** One root-owned notification connection; cache coordination stays here. */
export async function syncMediaFacts(api: ApiClientContract, client: QueryClient, signal: AbortSignal): Promise<void> {
  let delay = 1
  while (!signal.aborted) {
    try {
      for await (const event of api.streamMediaFactsEvents(signal)) {
        if (signal.aborted) return
        if (event.type !== 'changed') continue
        delay = 1
        await Promise.all([
          refreshOrRemoveQueries(client, queryKeys.works.contentPrefix()),
          refreshOrRemoveQueries(client, queryKeys.libraries.memberTreesAll()),
        ])
      }
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) return
    }
    if (signal.aborted) return
    try {
      await backOff(delay, signal)
    } catch {
      return
    }
    delay = Math.min(delay * 2, 30)
  }
}

export function useMediaFactsSync(): void {
  const api = useApiClient()
  const client = useQueryClient()
  const controller = new AbortController()
  onMounted(() => { void syncMediaFacts(api, client, controller.signal) })
  onBeforeUnmount(() => controller.abort())
}
