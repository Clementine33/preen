import type { LyricsWindow } from '@/composables/use-lyrics-window'
import { watchEffect } from 'vue'
import { useKeymap } from '@/composables/use-keymap'
import { usePlayerStore } from '@/stores/player'

/**
 * The player's global controls, installed once by the app: its keys (live
 * whenever something is loaded, on every page) and the system media session —
 * hardware media keys and lock-screen controls. No artwork: the queue carries no cover address.
 */
export function usePlayerKeys(lyricsWindow?: LyricsWindow): void {
  const player = usePlayerStore()
  const seekBy = (seconds: number): void => {
    if (player.rj !== null)
      player.seek(Math.min(Math.max(player.elapsed + seconds, 0), player.duration ?? Infinity))
  }
  useKeymap({
    'player.toggle': () => player.togglePause(),
    'player.seek': event => seekBy(event.key === 'ArrowRight'
      ? 10
      : -10),
    'player.volume': event => player.setVolume(Math.round((player.volume + (event.key === 'ArrowUp'
      ? 0.1
      : -0.1)) * 100) / 100),
    'player.mute': () => player.toggleMute(),
    'player.step': event => player.step(event.key === '.'
      ? 1
      : -1),
    ...(lyricsWindow?.supported
      ? {
          'player.lyrics': () => lyricsWindow.toggle(),
        }
      : {}),
  }, () => player.rj !== null)

  const session = navigator.mediaSession
  if (!session)
    return
  watchEffect(() => {
    session.metadata = player.rj === null
      ? null
      : new MediaMetadata({ title: player.title, album: player.rj })
    session.playbackState = player.rj === null
      ? 'none'
      : player.paused ? 'paused' : 'playing'
  })
  session.setActionHandler('play', () => player.paused && player.togglePause())
  session.setActionHandler('pause', () => !player.paused && player.togglePause())
  session.setActionHandler('previoustrack', () => player.step(-1))
  session.setActionHandler('nexttrack', () => player.step(1))
  session.setActionHandler('seekbackward', d => seekBy(-(d.seekOffset ?? 10)))
  session.setActionHandler('seekforward', d => seekBy(d.seekOffset ?? 10))
  session.setActionHandler('seekto', d => d.seekTime != null && player.rj !== null && player.seek(d.seekTime))
}
