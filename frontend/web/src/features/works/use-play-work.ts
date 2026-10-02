import { useQueryClient } from '@tanstack/vue-query'
import { deriveTracks, playableCopy, trackQueue } from '@/features/work-detail/derive'
import { useApiClient } from '@/lib/api/client'
import type { WorkContent } from '@/lib/api/types'
import { queryKeys } from '@/queries/query-keys'
import { usePlayerStore } from '@/stores/player'

// Playing a work from the wall: its tracks queue in their first format, from
// the copy the detail page would read. A work without an audio file is one
// silent entry under its title (ADR 0012 §4).

interface PlayableWork {
  rj: string
  title: string
}

export function usePlayWork() {
  const api = useApiClient()
  const client = useQueryClient()
  const player = usePlayerStore()

  /** Synchronous, for a click with the manifest in hand: play() stays user-initiated. */
  function playFrom(work: PlayableWork, content: WorkContent | null, index = 0): void {
    const copy = playableCopy(content)
    const queue = copy?.tree
      ? trackQueue(deriveTracks(copy.tree), '', (file) =>
          api.workFileUrl({ rj: work.rj, copyId: copy.copy_id, relPath: file.rel_path }))
      : []
    if (queue.length > 0) player.play(work.rj, queue, index)
    else player.play(work.rj, [{ title: work.title }])
  }

  /**
   * The work's first track, the manifest fetched if the inspector has not
   * already. The one await stays well inside the click's user activation.
   */
  async function playWork(work: PlayableWork): Promise<void> {
    const content = await client.ensureQueryData({
      queryKey: queryKeys.works.content(work.rj),
      queryFn: ({ signal }) => api.getWorkContent(work.rj, signal),
    }).catch(() => undefined)
    // An unreadable manifest plays nothing: the silent clock would pretend audio exists.
    if (content !== undefined) playFrom(work, content)
  }

  return { playFrom, playWork }
}
