import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import { onBeforeUnmount, onMounted } from 'vue'
import { ApiError, backOff, useApiClient } from '@/lib/api/client'
import type { ApiClientContract } from '@/lib/api/types'
import { refreshOrRemoveQueries } from '@/queries/cache-sync'
import { queryKeys } from '@/queries/query-keys'
import { sweepWorkCaches } from '@/queries/works'

/** A burst of commits reloads once: quiet for this long, or never later than the ceiling. */
export const WORKS_DEBOUNCE_MS = 1000
export const WORKS_MAX_WAIT_MS = 5000

/** One root-owned notification connection; cache coordination stays here. */
export async function syncMediaFacts(api: ApiClientContract, client: QueryClient, signal: AbortSignal): Promise<void> {
  let delay = 1
  let timer: ReturnType<typeof setTimeout> | null = null
  let firstPendingAt = 0
  const reloadWorks = () => {
    timer = null
    if (!signal.aborted) void sweepWorkCaches(client)
  }
  const scheduleWorks = () => {
    const now = Date.now()
    if (timer === null) firstPendingAt = now
    else clearTimeout(timer)
    timer = setTimeout(reloadWorks, Math.min(WORKS_DEBOUNCE_MS, firstPendingAt + WORKS_MAX_WAIT_MS - now))
  }
  signal.addEventListener('abort', () => { if (timer !== null) clearTimeout(timer) }, { once: true })
  while (!signal.aborted) {
    try {
      for await (const event of api.streamMediaFactsEvents(signal)) {
        if (signal.aborted) return
        if (event.type === 'works_changed') {
          delay = 1
          scheduleWorks()
          continue
        }
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
