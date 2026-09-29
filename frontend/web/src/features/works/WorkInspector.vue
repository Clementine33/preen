<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { REDUCED_MOTION_QUERY, useMediaQuery } from '@/composables/use-media-query'
import type { WorkDetail } from '@/lib/api/types'
import { useWorkDetail } from '@/queries/works'
import { usePlayerStore } from '@/stores/player'

// The inspector: the detail of the selected work, rendered to fit 1440x900
// without scrolling (ADR 0012 §4). While cruising the wall the text follows
// immediately, but the cover/SPEC/TRACKLIST dim to 30% and are swapped only
// after a 70ms pause, so fast browsing merges updates instead of flickering.

const props = defineProps<{
  rj: string | null
  /** On the detail page: no wall to return to, so no OPEN DETAIL and no wall keys. */
  standalone?: boolean
}>()

const router = useRouter()
const player = usePlayerStore()
const { work } = useWorkDetail(() => props.rj)

/** The detail of the work actually being displayed, or null while loading. */
const workNow = computed(() => (work.value && props.rj !== null && work.value.rj === props.rj ? work.value : null))

// ── Deferred body: cover + SPEC + TRACKLIST swap after a 70ms pause. ──
const deferredWork = ref<WorkDetail | null>(null)
const dim = ref(false)
let detailTimer: number | null = null
const reducedMotion = useMediaQuery(REDUCED_MOTION_QUERY)

// Post-flush so the scramble runs after Vue has patched the title's text node:
// the animation writes textContent frame by frame, and a pre-flush watcher
// would have its first frame overwritten by the patch.
watch(
  workNow,
  (current) => {
    if (!current) {
      // Still loading the next work: keep the last body, dimmed, so browsing
      // faster than the detail arrives never flashes an empty plate. Only an
      // empty selection clears it.
      if (props.rj === null) {
        deferredWork.value = null
        dim.value = false
      } else if (deferredWork.value !== null) {
        dim.value = true
      }
      return
    }
    if (reducedMotion.value) {
      deferredWork.value = current
      scramble(current.title ?? '')
      return
    }
    dim.value = true
    if (detailTimer !== null) window.clearTimeout(detailTimer)
    detailTimer = window.setTimeout(() => {
      deferredWork.value = current
      dim.value = false
    }, 70)
    scramble(current.title ?? '')
  },
  { flush: 'post' },
)
onBeforeUnmount(() => {
  if (detailTimer !== null) window.clearTimeout(detailTimer)
  cancelAnimationFrame(scrambleFrame)
})

// ── Title: resolves left to right like typing, with a scrambled leading edge
// 3 characters wide. Duration min(260, 120 + n*5) ms. ──
const GLYPHS = 'アイウエオカキクケコサシスセソタチツテトナニヌネノ'
const titleEl = ref<HTMLElement | null>(null)
let scrambleFrame = 0
function scramble(text: string) {
  cancelAnimationFrame(scrambleFrame)
  if (!titleEl.value) return
  if (reducedMotion.value || text === '') {
    titleEl.value.textContent = text
    return
  }
  const chars = [...text]
  const head = 3
  const duration = Math.min(260, 120 + chars.length * 5)
  const started = performance.now()
  const tick = (now: number) => {
    if (!titleEl.value) return
    const progress = Math.min(1, (now - started) / duration)
    const done = Math.floor(chars.length * progress)
    const noise = chars
      .slice(done, done + head)
      .map(() => GLYPHS[(Math.random() * GLYPHS.length) | 0])
      .join('')
    titleEl.value.textContent = chars.slice(0, done).join('') + (progress < 1 ? noise : '')
    if (progress < 1) scrambleFrame = requestAnimationFrame(tick)
  }
  scrambleFrame = requestAnimationFrame(tick)
}

// ── Cover plate: two layers; the new cover wipes in over the old one in
// 240ms, and four corner ticks frame the specimen. ──
const coverBack = ref<HTMLElement | null>(null)
const coverFront = ref<HTMLElement | null>(null)
let shownCover: string | null = null
watch(
  () => (deferredWork.value ? deferredWork.value.cover_url ?? deferredWork.value.thumbnail_url : null),
  (url) => {
    if (url === shownCover || !coverBack.value || !coverFront.value) return
    if (reducedMotion.value) {
      // No wipe, but the cover still has to be painted.
      shownCover = url
      paintCover(coverFront.value, url)
      return
    }
    paintCover(coverBack.value, shownCover)
    shownCover = url
    paintCover(coverFront.value, url)
    // Replay the wipe: clip to fully-open, force a reflow, then run.
    coverFront.value.classList.remove('run')
    coverFront.value.classList.add('enter')
    void coverFront.value.offsetWidth
    coverFront.value.classList.add('run')
    coverFront.value.classList.remove('enter')
  },
)
function paintCover(el: HTMLElement, url: string | null) {
  el.classList.toggle('hatch', url === null)
  el.style.backgroundImage = url === null ? '' : `url("${url}")`
}

/** `YYYY.MM.DD` from the catalog's `YYYY-MM-DD`. */
function dottedDate(value: string | null): string {
  return value ? value.replaceAll('-', '.') : '—'
}

const voice = computed(() =>
  deferredWork.value && deferredWork.value.voice_actors.length > 0
    ? deferredWork.value.voice_actors.map((actor) => actor.name).join(' / ')
    : '—',
)

// PLAY needs the loaded work: it plays what the inspector shows, by name.
const canPlay = computed(() => workNow.value !== null)
function playNowPlaying() {
  if (workNow.value) player.play({ rj: workNow.value.rj, title: workNow.value.title ?? workNow.value.rj })
}
function openDetail() {
  if (props.rj !== null) router.push({ name: 'work-detail', params: { rj: props.rj } })
}

const arrowIcon = 'M0 4.5h7.2L4.6 1.9l.7-.7L9.1 5 5.3 8.8l-.7-.7 2.6-2.6H0z'
const playIcon = 'M1 0l9 5-9 5z'
</script>

<template>
  <aside class="insp substrate-panel" aria-label="Inspector">
    <div class="strip">
      <span class="strip-name lbl">INSPECT</span>
      <span class="lbl">{{ rj ?? '—' }}</span>
    </div>

    <div class="plate">
      <div class="plate-frame">
        <div ref="coverBack" class="cover back"></div>
        <div ref="coverFront" class="cover front hatch"></div>
      </div>
      <i class="tick tl"></i><i class="tick tr"></i><i class="tick bl"></i><i class="tick br"></i>
    </div>

    <span class="insp-circle lbl">CIRCLE<b>{{ workNow?.circle ?? '—' }}</b></span>
    <h2 ref="titleEl" class="insp-title">{{ workNow?.title ?? '' }}</h2>

    <div class="strip"><span class="strip-name lbl">SPEC</span></div>
    <dl class="spec" :class="{ dim }">
      <div class="spec-row"><dt class="lbl">VOICE</dt><dd>{{ voice }}</dd></div>
      <div class="spec-row"><dt class="lbl">RELEASE</dt><dd>{{ dottedDate(deferredWork?.release_date ?? null) }}</dd></div>
      <div class="spec-row"><dt class="lbl">FORMAT</dt><dd>—</dd></div>
      <div class="spec-row"><dt class="lbl">SIZE</dt><dd>—</dd></div>
    </dl>

    <div class="strip">
      <span class="strip-name lbl">TRACKLIST</span>
      <span class="lbl">—</span>
    </div>
    <div class="tracks" :class="{ dim }">
      <p class="no-tracks lbl">NO TRACK DATA</p>
    </div>

    <div class="insp-foot">
      <div class="insp-foot-inner">
        <button class="btn btn-primary" type="button" :disabled="!canPlay" @click="playNowPlaying">
          <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="playIcon" /></svg>PLAY
        </button>
        <button v-if="!standalone" class="btn" type="button" :disabled="rj === null" @click="openDetail">
          OPEN DETAIL<svg viewBox="0 0 10 10" aria-hidden="true"><path :d="arrowIcon" /></svg>
        </button>
      </div>
      <div v-if="!standalone" class="keys lbl"><kbd>J</kbd><kbd>K</kbd><kbd>←↑↓→</kbd> MOVE<span class="sep">·</span><kbd>P</kbd>
        PLAY<span class="sep">·</span><kbd>␣</kbd> PAUSE<span class="sep">·</span><kbd>↵</kbd> OPEN</div>
    </div>
  </aside>
</template>

<style scoped>
.insp {
  border-left: 1px solid var(--wire-mid);
  overflow-y: auto; min-height: 0;
  display: flex; flex-direction: column;
  padding: var(--sp-5) var(--sp-6) var(--sp-5);
}

/* The specimen plate: a 4:3 cover never scaled above its native 560px. */
.plate {
  position: relative; width: 100%; max-width: var(--cover-native);
  margin: var(--sp-4) 0 var(--sp-3);
}
.plate-frame { position: relative; width: 100%; aspect-ratio: var(--cover-ratio); }
.cover {
  position: absolute; inset: 0; overflow: hidden;
  background-size: cover; background-position: center;
  box-shadow: inset 0 0 0 1px var(--wire-fine);
}
.cover.front { clip-path: inset(0 0 0 0); }
.cover.front.enter { clip-path: inset(0 100% 0 0); }
.cover.front.run { transition: clip-path 240ms var(--ease-out); }
.tick { position: absolute; width: 8px; height: 8px; border-color: var(--wire-strong); border-style: solid; border-width: 0; }
.tick.tl { left: -4px; top: -4px; border-left-width: 1px; border-top-width: 1px; }
.tick.tr { right: -4px; top: -4px; border-right-width: 1px; border-top-width: 1px; }
.tick.bl { left: -4px; bottom: -4px; border-left-width: 1px; border-bottom-width: 1px; }
.tick.br { right: -4px; bottom: -4px; border-right-width: 1px; border-bottom-width: 1px; }

.insp-circle { display: block; }
.insp-circle b { color: var(--fg-base); font-weight: 600; }
.insp-title {
  margin: var(--sp-1) 0 var(--sp-4);
  font-family: var(--font-mincho);
  font-size: var(--fs-l); line-height: var(--lh-l); font-weight: 600;
  /* Japanese never carries tracking (ADR 0012 §2). */
  letter-spacing: 0; min-height: var(--lh-l);
}

.spec {
  display: grid; grid-template-columns: 1fr 1fr; column-gap: var(--sp-5);
  margin: var(--sp-1) 0 var(--sp-4);
}
.spec-row {
  display: flex; justify-content: space-between; align-items: baseline; gap: var(--sp-2);
  height: 28px; padding-top: 8px; border-bottom: 1px solid var(--wire-fine);
}
.spec-row dd {
  font-size: var(--fs-s); line-height: var(--lh-s); font-weight: 500;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.spec, .tracks { transition: opacity 80ms linear; }
.dim { opacity: .3; }

.tracks { margin-top: var(--sp-2); }
.no-tracks {
  display: flex; align-items: center; height: 28px;
  padding: 0 var(--sp-2); border-bottom: 1px solid var(--wire-fine);
  letter-spacing: .04em; text-transform: none;
}

.insp-foot { margin-top: auto; padding-top: var(--sp-4); }
.insp-foot-inner {
  border-top: 2px solid var(--wire-strong);
  padding-top: var(--sp-4);
  display: flex; justify-content: space-between; align-items: center; gap: var(--sp-3);
}
.insp-foot .btn:disabled { cursor: not-allowed; opacity: .5; }
.keys { margin-top: var(--sp-2); }
.keys kbd {
  font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted);
  border: 1px solid var(--wire-fine); padding: 0 4px; margin-right: 2px;
}
</style>
