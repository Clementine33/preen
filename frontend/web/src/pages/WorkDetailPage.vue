<script setup lang="ts">
import { useApiClient } from '@/lib/api/client'
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { canonicalRj } from '@/app/route-params'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import PlayerBar from '@/features/works/PlayerBar.vue'
import WorkBay from '@/features/work-detail/WorkBay.vue'
import WorkDeck from '@/features/work-detail/WorkDeck.vue'
import WorkLeaf from '@/features/work-detail/WorkLeaf.vue'
import WorkViewer from '@/features/work-detail/WorkViewer.vue'
import { useWorkContent, useWorkDetail } from '@/queries/works'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'

// The work detail page: a 48px header over a stage of deck
// and leaf with the file bay docked below or pinned right, and the player
// baseline at the very bottom. All of it is one session in the work-detail
// store; the address carries exactly one fact, the work number — no query
// survives the route (address hygiene keeps none for it).

const route = useRoute()
const router = useRouter()
const store = useWorkDetailStore()
const player = usePlayerStore()

/** The work number, canonical: the route may carry the 6-digit spelling. */
const rj = computed(() => {
  const raw = route.params.rj
  return typeof raw === 'string' && raw !== '' ? canonicalRj(raw) : null
})

const api = useApiClient()
const { work } = useWorkDetail(rj)
const { content } = useWorkContent(rj)

// A different work is a new session, dropped the moment the route changes;
// the manifest of the new work rehydrates when it lands.
watch([rj, content], ([id, now]) => {
  if (store.rj !== id) store.newVisit()
  if (now !== null && id !== null && now.rj === id) store.bind(id, now, (ref) => api.workFileUrl(ref))
}, { immediate: true })

// Prev/next and a track's end move the player on; the bay and the leaf follow.
watch(() => player.trackKey, (key) => {
  if (key === null || player.rj !== store.rj) return
  const track = store.tracks.findIndex((t) => t.key === key)
  if (track >= 0 && track !== store.track) store.followTrack(track)
})

// Back to the wall (or collection) this work was opened from: that history
// entry holds its whole address and where it was scrolled, so stepping back
// restores both. Arriving any other way, the wall opens on this work's number alone.
function backToWall(): void {
  const back = router.options.history.state.back
  if (typeof back === 'string' && /^\/(works|collections\/[^/]+)$/.test(back.split('?')[0]!)) {
    router.back()
    return
  }
  void router.push({ name: 'works', query: rj.value !== null ? { rj: rj.value } : {} })
}

// ── Keyboard: I flips, the arrows step (the viewer shares
//    the same set/idx), Esc closes the topmost layer, Space only pauses. ──

/** A text field or an open menu keeps every key it receives. */
function ownsKeys(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.closest('[data-own-keys]') !== null ||
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    target.isContentEditable
  )
}

/** Enter and Space already mean something on a link or a button. */
function activates(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.closest('a, button') !== null
}

function onKeydown(event: KeyboardEvent): void {
  if (ownsKeys(event.target)) return
  if (event.metaKey || event.ctrlKey || event.altKey) return
  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
    event.preventDefault()
    store.step(event.key === 'ArrowRight' ? 1 : -1)
    return
  }
  if (event.key === 'i' || event.key === 'I') {
    store.flip()
    return
  }
  if (event.key === 'Escape') {
    // The viewer is a native dialog: the browser closes it on Esc; the page
    // only ever closes the layer underneath the topmost one.
    if (store.viewerOpen) return
    if (store.dock === 'bottom' && store.bayOpen) store.closeBay()
    return
  }
  if (activates(event.target)) return
  if (event.key === ' ') {
    event.preventDefault()
    player.togglePause()
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <section class="detail substrate" :data-dock="store.dock" aria-label="作品详情">
    <header class="header">
      <div class="header-left">
        <GlobalMenu />
        <button class="btn btn-s" type="button" @click="backToWall">
          <svg viewBox="0 0 10 10" aria-hidden="true" style="transform: scaleX(-1)"><path d="M0 4.5h7.2L4.6 1.9l.7-.7L9.1 5 5.3 8.8l-.7-.7 2.6-2.6H0z" /></svg>BACK
        </button>
        <span class="lbl">{{ rj ?? '—' }}</span>
        <span class="h-title" :lang="work?.relation?.language === 'CHI_HANS' ? 'zh-Hans' : work?.relation?.language === 'CHI_HANT' ? 'zh-Hant' : 'ja'">{{ work?.title ?? '' }}</span>
      </div>
    </header>

    <main class="main">
      <section class="stage" :class="{ read: store.leaf === 'script' }">
        <WorkDeck />
        <WorkLeaf />
      </section>
      <WorkBay />
    </main>

    <PlayerBar />
    <WorkViewer />
  </section>
</template>

<style scoped>
/* App rows: header 48, main area 1fr, bottom bar 28. The two outer edges carry
   the only double bar; everything inside uses single 1px lines. */
.detail {
  display: grid; grid-template-rows: 48px minmax(0, 1fr) auto;
  width: 100%; height: 100%;
  font-variant-numeric: tabular-nums;
}

.header { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-4); padding: 0 var(--sp-6); position: relative; min-width: 0; }
.header::after { content: ''; position: absolute; left: var(--sp-6); right: var(--sp-6); bottom: 0; height: 4px; border-top: 1px solid var(--wire-fine); border-bottom: 2px solid var(--wire-strong); }
.header-left { display: flex; align-items: center; gap: var(--sp-2); min-width: 0; }
.h-title { font-family: var(--font-mincho); font-size: var(--fs-m); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; letter-spacing: 0; }
.header-meta { display: flex; align-items: center; gap: var(--sp-2); flex-shrink: 0; }

/* ── The stage and the bay: bottom dock = deck beside leaf, bay lifting below;
   right dock = the bay pinned as a column and the stage stacked deck over
   leaf (v3's re-stack). ── */
.main { display: grid; min-height: 0; min-width: 0; }
[data-dock='bottom'] .main { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr) auto; }
[data-dock='right'] .main { grid-template-columns: minmax(0, 1fr) clamp(360px, 30vw, 460px); grid-template-rows: minmax(0, 1fr); }

.stage { display: grid; gap: var(--sp-6); padding: var(--sp-6); min-height: 0; min-width: 0; }
[data-dock='bottom'] .stage { grid-template-columns: minmax(0, 1.15fr) minmax(340px, .85fr); grid-template-rows: minmax(0, 1fr); }
[data-dock='bottom'] .stage.read { grid-template-columns: 240px minmax(0, 1fr); }
[data-dock='right'] .stage { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1.25fr) minmax(0, 1fr); }
[data-dock='right'] .stage.read { grid-template-rows: 180px minmax(0, 1fr); }

@media (max-width: 768px) {
  .header { padding: 0 var(--sp-4); }
  .header::after { left: var(--sp-4); right: var(--sp-4); }
  .h-title { display: none !important; }
  [data-dock='bottom'] .main, [data-dock='right'] .main { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr) auto; }
  [data-dock='bottom'] .stage, [data-dock='bottom'] .stage.read,
  [data-dock='right'] .stage, [data-dock='right'] .stage.read {
    grid-template-columns: minmax(0, 1fr); grid-template-rows: auto auto;
    overflow-y: auto; padding: var(--sp-4); gap: var(--sp-5); align-content: start;
  }
  .stage > .deck, .stage > :deep(.leaf) { min-height: auto; }
  .stage.read :deep(.leaf-body) { height: 60vh; overflow: hidden; }
}
</style>
