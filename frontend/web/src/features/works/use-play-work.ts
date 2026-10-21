import { useQueryClient } from '@tanstack/vue-query'
import { deriveTracks, pickFormat, playableCopy, playQueue, resumePoint, trackFormats } from '@/features/work-detail/derive'
import { useApiClient } from '@/lib/api/client'
import type { WorkContent, WorkListening } from '@/lib/api/types'
import { usePreferences } from '@/queries/preferences'
import { queryKeys } from '@/queries/query-keys'
import { usePlayerStore } from '@/stores/player'

// Playing a work from the wall: its tracks queue in their effective format (the
// Format Preference, else every format merged), from the copy the detail page would read. A work without an audio file is one
// silent entry under its title. Playing a work from the wall continues its
// Listening Position when it has a usable one.

interface PlayableWork {
  rj: string
  title: string
}

export function usePlayWork() {
  const api = useApiClient()
  const client = useQueryClient()
  const player = usePlayerStore()
  const { trackFormats: preference } = usePreferences()

  /** Synchronous, for a click with the manifest in hand: play() stays user-initiated. */
  function playFrom(work: PlayableWork, content: WorkContent | null, index = 0, format = '', resumeAt?: number): void {
    const copy = playableCopy(content)
    const tracks = copy?.tree ? deriveTracks(copy.tree) : []
    // No explicit format: the preference decides, so no caller repeats it.
    format ||= pickFormat(trackFormats(tracks), preference.value) ?? ''
    // A codec that the requested start track lacks starts at the first track that has it;
    // a resume keeps its track and plays it in the track's first format instead.
    const at = resumeAt === undefined && format && !tracks[index]?.files[format] ? Math.max(tracks.findIndex((t) => t.files[format]), 0) : index
    const { queue, index: start } = playQueue(tracks, at, format, (file) =>
      api.workFileUrl({ rj: work.rj, copyId: copy!.copy_id, relPath: file.rel_path }), copy ? { copyId: copy.copy_id } : undefined)
    if (queue.length > 0) player.play(work.rj, queue, start, resumeAt)
    else player.play(work.rj, [{ title: work.title }])
  }

  /** The saved rows, fresh: the cached detail predates the player's latest write. */
  async function savedListening(rj: string): Promise<WorkListening[]> {
    try {
      return (await api.getWork(rj)).listening
    } catch {
      return []
    }
  }

  /**
   * The work from its Listening Position, else its first track; the manifest
   * is fetched if the inspector has not already. The awaits stay well inside
   * the click's user activation.
   */
  async function playWork(work: PlayableWork, format = ''): Promise<void> {
    const [content, saved] = await Promise.all([
      client.ensureQueryData({
        queryKey: queryKeys.works.content(work.rj),
        queryFn: ({ signal }) => api.getWorkContent(work.rj, signal),
      }).catch(() => undefined),
      savedListening(work.rj),
    ])
    // An unreadable manifest plays nothing: the silent clock would pretend audio exists.
    if (content === undefined) return
    const copy = playableCopy(content)
    const point = resumePoint(saved, copy?.tree ? deriveTracks(copy.tree) : [])
    if (point) playFrom(work, content, point.track, format, point.position)
    else playFrom(work, content, 0, format)
  }

  return { playFrom, playWork }
}
