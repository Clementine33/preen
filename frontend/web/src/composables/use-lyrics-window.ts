import { computed, inject, provide, ref, type InjectionKey } from 'vue'
import { useTimedLyrics } from '@/composables/use-timed-lyrics'
import { useDesktopAdapter, type LyricsCommand, type LyricsFrame } from '@/lib/desktop/desktop-adapter'
import { usePlayerStore } from '@/stores/player'

/**
 * The Lyrics Window's owner: whether it is open, and the frame it is fed. The
 * frame follows the player's current queue item on any page and updates from
 * `player.elapsed` (the audio element's timeupdate), never from the main
 * window's animation frames, so a background tab keeps it current. Commands
 * come back as plain player actions. The window outlives tracks without
 * lyrics and an idle player; only the user (or the page unloading) closes it.
 */
export function createLyricsWindow() {
  const adapter = useDesktopAdapter()
  const player = usePlayerStore()
  const item = computed(() => (player.rj === null ? undefined : player.queue[player.trackIndex]))
  const source = computed(() => (player.rj !== null && item.value?.lyrics ? { rj: player.rj, ...item.value.lyrics } : null))
  const { lines, nowIndex } = useTimedLyrics(source, () => player.elapsed)

  // Whether the line on show arrived by stepping to the next one; fixed per line, since frame() runs on every render.
  let previous = { track: '', index: -1, key: '', advance: false }
  function advanced(track: string, index: number, key: string): boolean {
    if (key === previous.key) return previous.advance
    const advance = track === previous.track && index === previous.index + 1
    previous = { track, index, key, advance }
    return advance
  }

  const frame = (): LyricsFrame => {
    const track = `${player.rj}:${player.trackIndex}`
    // A WebVTT gap keeps the last cue up, dimmed; an LRC line already holds until the next.
    const last = lines.value.reduce((at, line, i) => (player.elapsed >= line.time ? i : at), -1)
    const shown = lines.value[last]
    if (!shown) previous = { track, index: -1, key: '', advance: false }
    return {
      rj: player.rj,
      title: player.title,
      paused: player.paused,
      hasPrev: player.hasPrev,
      hasNext: player.hasNext,
      line: shown ? { key: `${track}:${last}`, text: shown.text, dim: last !== nowIndex.value, advance: advanced(track, last, `${track}:${last}`) } : null,
      noLyrics: player.rj !== null && !item.value?.lyrics,
    }
  }
  const commands: Record<LyricsCommand, () => void> = {
    prev: () => player.step(-1),
    toggle: () => player.togglePause(),
    next: () => player.step(1),
  }

  const open = ref(false)
  let close: (() => void) | null = null

  /** Open or close. Opening needs a user gesture and must not await before asking the adapter. */
  function toggle(): void {
    if (close) return close()
    if (open.value || !item.value?.lyrics) return
    open.value = true
    void adapter.openLyricsWindow(frame, (command) => commands[command](), () => {
      open.value = false
      close = null
    }).then((closer) => {
      if (closer) close = closer
      else open.value = false
    })
  }

  return {
    supported: adapter.lyricsWindow,
    open,
    /** Opening needs Timed Lyrics on the current track; closing is always possible. */
    available: computed(() => open.value || !!item.value?.lyrics),
    toggle,
  }
}

export type LyricsWindow = ReturnType<typeof createLyricsWindow>
export const lyricsWindowKey: InjectionKey<LyricsWindow> = Symbol('lyrics-window')

/** Called once by the app; returns the instance because a component cannot inject its own provide. */
export function provideLyricsWindow(): LyricsWindow {
  const lyricsWindow = createLyricsWindow()
  provide(lyricsWindowKey, lyricsWindow)
  return lyricsWindow
}

export const useLyricsWindow = (): LyricsWindow | null => inject(lyricsWindowKey, null)
