import { useQueryClient } from '@tanstack/vue-query'
import { watch } from 'vue'
import { useApiClient } from '@/lib/api/client'
import type { WorkDetail } from '@/lib/api/types'
import { refreshOrRemoveQueries } from '@/queries/cache-sync'
import { queryKeys } from '@/queries/query-keys'
import { usePlayerStore } from '@/stores/player'

/**
 * Installed once by the app: gives the player the client it writes the
 * Listening Position through, and keeps the work caches in step with what the
 * server acknowledged. The detail is patched with the row the PUT returned, so
 * a save costs no extra read; the wall is refetched only when a work starts,
 * finishes or is cleared, since its summary is the server's to derive and
 * refetching every 15 s would reorder a 最近收听 sort under the reader.
 */
export function useListeningSync(): void {
  const player = usePlayerStore()
  const client = useQueryClient()
  player.bindClient(useApiClient())
  watch(() => player.saved, (saved) => {
    if (!saved) return
    const { rj, row } = saved
    client.setQueryData<WorkDetail>(queryKeys.works.detail(rj), (detail) => detail && {
      ...detail,
      listening: row ? [row, ...detail.listening.filter((l) => l.rendition !== row.rendition)] : [],
    })
    if (saved.lists) void refreshOrRemoveQueries(client, queryKeys.works.listPrefix())
  })
}
