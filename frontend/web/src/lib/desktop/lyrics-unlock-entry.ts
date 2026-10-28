import { Call } from '@wailsio/runtime'
import { createApp, h } from 'vue'
import LyricsUnlockView from '@/features/lyrics-window/LyricsUnlockView.vue'

// The desktop unlock chip's page (ADR 0016): main.ts routes /lyrics-unlock
// here before any application bootstrap. The shell floats it over a locked
// Lyrics Window; its one button asks the shell to unlock, which also removes it.

export function mountLyricsUnlock(): void {
  const app = createApp({
    render: () => h(LyricsUnlockView, {
      onUnlock: () => void Call.ByName('main.Desktop.SetLyricsLocked', false),
    }),
  })
  app.mount('#app')
}
