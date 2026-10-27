import type { MaybeRefOrGetter } from 'vue'
import type { LrcLine } from '@/features/work-detail/derive'
import { computed, ref, toValue, watch } from 'vue'
import { parseLyrics } from '@/features/work-detail/derive'
import { useApiClient } from '@/lib/api/client'

/** Where a track's Timed Lyrics file lives. */
export interface LyricsSource {
  rj: string
  copyId: string
  relPath: string
}

/**
 * The Timed Lyrics of `source`, fetched and parsed whenever it changes, and the
 * line `clock` (seconds into the track) is on. Shared by the detail page's
 * LISTEN pane and the Lyrics Window.
 */
export function useTimedLyrics(source: MaybeRefOrGetter<LyricsSource | null>, clock: () => number) {
  const api = useApiClient()
  const lines = ref<LrcLine[]>([])
  const status = ref('')

  watch(
    () => {
      const s = toValue(source)
      return s && `${s.rj}\n${s.copyId}\n${s.relPath}`
    },
    async (_, __, onCleanup) => {
      const controller = new AbortController()
      onCleanup(() => controller.abort())
      lines.value = []
      status.value = ''
      const s = toValue(source)
      if (!s)
        return
      status.value = '正在读取歌词…'
      let text: string
      try {
        text = await api.getWorkText(s, controller.signal)
      }
      catch {
        if (!controller.signal.aborted)
          status.value = '无法读取歌词文件'
        return
      }
      if (controller.signal.aborted)
        return
      lines.value = parseLyrics(text, s.relPath)
      status.value = lines.value.length ? '' : '未找到有效的时间轴歌词'
    },
    { immediate: true },
  )

  /** The last line at or before the clock — what "now" is. */
  const nowIndex = computed(() => {
    const now = clock()
    let index = -1
    lines.value.forEach((line, i) => {
      if (now >= line.time && (line.endTime === undefined || now < line.endTime))
        index = i
    })
    return index
  })

  return { lines, status, nowIndex }
}
