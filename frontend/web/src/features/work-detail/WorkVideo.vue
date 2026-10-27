<script setup lang="ts">
import type { ContentFileRef } from '@/lib/api/types'
import { computed, ref, watch } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { useWorkDetailStore } from '@/stores/work-detail'

// The video modal: a native <dialog> around a native <video controls>, opened
// by a video row of the FILES tab. The browser decodes whatever it can; an
// unsupported codec shows a message. Closing stops the video and leaves the
// bottom player paused.

const store = useWorkDetailStore()
const api = useApiClient()
const file = computed(() => store.video)
const src = computed(() => {
  const item = file.value
  if (!item || !store.rj || !store.activeCopy)
    return ''
  const ref: ContentFileRef = { rj: store.rj, copyId: store.activeCopy.copy_id, relPath: item.rel_path }
  return api.workFileUrl(ref)
})

// jsdom has no showModal, so tests render the same DOM by the plain open attribute.
const dialogEl = ref<HTMLDialogElement | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)
const failed = ref(false)
watch(
  () => store.video,
  (video) => {
    failed.value = false
    const el = dialogEl.value
    if (!el)
      return
    if (video && !el.open) {
      if (typeof el.showModal === 'function')
        el.showModal()
      else el.setAttribute('open', '')
    }
    else if (!video && el.open) {
      if (typeof el.close === 'function')
        el.close()
      else el.removeAttribute('open')
    }
  },
)

// Dropping the src is what stops the download; hiding the dialog alone would not.
function onClose(): void {
  const video = videoEl.value
  if (video) {
    video.pause()
    video.removeAttribute('src')
    video.load()
  }
  store.closeVideo()
}

function close(): void {
  const el = dialogEl.value
  if (el && typeof el.close === 'function') {
    el.close()
  }
  else {
    el?.removeAttribute('open')
    onClose()
  }
}
</script>

<template>
  <dialog ref="dialogEl" class="video-modal" aria-label="视频播放" @close="onClose">
    <div class="strip">
      <span class="strip-name lbl">VIDEO</span>
      <span class="strip-side">
        <span class="strip-file" lang="ja">{{ file?.name ?? '—' }}</span>
        <button class="btn btn-s" type="button" @click="close">CLOSE</button>
      </span>
    </div>
    <div class="vd-body">
      <video v-if="file" ref="videoEl" :src="src" controls autoplay @error="failed = true" />
      <p v-if="failed" class="vd-error">浏览器无法播放</p>
    </div>
  </dialog>
</template>

<style scoped>
.video-modal { position: fixed; inset: 0; width: 100vw; height: 100vh; max-width: none; max-height: none; margin: 0; padding: 0; border: 0; background: var(--grain), var(--bg-canvas); color: var(--fg-base); display: none; grid-template-rows: auto minmax(0, 1fr); }
.video-modal[open] { display: grid; }
.video-modal::backdrop { background: var(--bg-canvas); }
.video-modal .strip { margin: var(--sp-3) var(--sp-4) 0; }
.video-modal .strip-side { display: flex; align-items: center; gap: var(--sp-3); }
.strip-file { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-base); letter-spacing: .04em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.vd-body { position: relative; display: grid; place-items: center; min-height: 0; }
.vd-body video { max-width: calc(100vw - 112px); max-height: calc(100vh - 96px); box-shadow: 0 0 0 1px var(--wire-mid), 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid); }
.vd-error { position: absolute; font-family: var(--font-mono); font-size: var(--fs-s); color: var(--fg-muted); }
</style>
