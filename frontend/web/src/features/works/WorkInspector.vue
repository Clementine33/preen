<script setup lang="ts">
import type { WorkDetail } from '@/lib/api/types'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useReducedMotion, useSwitchBurst } from '@/composables/use-motion'
import { useCoverSrc } from '@/features/works/use-cover-src'
import { usePlayWork } from '@/features/works/use-play-work'
import WorkTracklist from '@/features/works/WorkTracklist.vue'
import { useWorkDetail } from '@/queries/works'
import { useLibraryUiStore } from '@/stores/library-ui'

// The inspector: the detail of the selected work, rendered to fit 1440x900
// without scrolling. While cruising the wall the text follows
// immediately, but the cover/SPEC/TRACKLIST dim to 30% and are swapped only
// after a 70ms pause, so fast browsing merges updates instead of flickering.
// The first fill (nothing shown yet, e.g. a restored `?rj=`) has nothing to
// merge or wipe over: it skips the pause, the title scramble and the wipe,
// and its cover is fetched at high priority.

const props = defineProps<{
  rj: string | null
  /** On the detail page: no wall to return to, so no OPEN DETAIL. */
  standalone?: boolean
}>()

const router = useRouter()
const { work } = useWorkDetail(() => props.rj)

/** The detail of the work actually being displayed, or null while loading. */
const workNow = computed(() => (work.value && props.rj !== null && work.value.rj === props.rj
  ? work.value
  : null))

// ── Deferred body: cover + SPEC + TRACKLIST swap after a 70ms pause. ──
// Seeded from the cache: a remount (the wall leaving and re-entering the
// desktop layout) finds the work already loaded, and no change would fill it.
const deferredWork = ref<WorkDetail | null>(workNow.value)
const dim = ref(false)
let detailTimer: number | null = null
const reducedMotion = useReducedMotion()
const { rapid: switching, note } = useSwitchBurst()

// Counted pre-flush, so the post-flush watcher below already sees the burst.
watch(() => props.rj, note)
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
      }
      else if (deferredWork.value !== null) {
        dim.value = true
      }
      return
    }
    // A refetch of the work on show (a saved position, a rating) is not a switch: swap quietly.
    if (current.rj === deferredWork.value?.rj) {
      if (detailTimer !== null)
        window.clearTimeout(detailTimer)
      deferredWork.value = current
      dim.value = false
      return
    }
    const first = deferredWork.value === null
    if (reducedMotion.value || switching.value || first) {
      if (detailTimer !== null)
        window.clearTimeout(detailTimer)
      deferredWork.value = current
      dim.value = false
      scramble(current.title ?? '', first)
      return
    }
    dim.value = true
    if (detailTimer !== null)
      window.clearTimeout(detailTimer)
    detailTimer = window.setTimeout(() => {
      deferredWork.value = current
      dim.value = false
    }, 70)
    scramble(current.title ?? '')
  },
  { flush: 'post' },
)
onBeforeUnmount(() => {
  if (detailTimer !== null)
    window.clearTimeout(detailTimer)
  cancelAnimationFrame(scrambleFrame)
})

// ── Title: resolves left to right like typing, with a scrambled leading edge
// 3 characters wide. Duration min(260, 120 + n*5) ms. ──
const GLYPHS = 'アイウエオカキクケコサシスセソタチツテトナニヌネノ'
const titleEl = ref<HTMLElement | null>(null)
let scrambleFrame = 0
function scramble(text: string, instant = false) {
  cancelAnimationFrame(scrambleFrame)
  if (!titleEl.value)
    return
  if (instant || reducedMotion.value || switching.value || text === '') {
    titleEl.value.textContent = text
    return
  }
  const chars = [...text]
  const head = 3
  const duration = Math.min(260, 120 + chars.length * 5)
  const started = performance.now()
  const tick = (now: number) => {
    if (!titleEl.value)
      return
    const progress = Math.min(1, (now - started) / duration)
    const done = Math.floor(chars.length * progress)
    const noise = chars
      .slice(done, done + head)
      .map(() => GLYPHS[(Math.random() * GLYPHS.length) | 0])
      .join('')
    titleEl.value.textContent = chars.slice(0, done).join('') + (progress < 1
      ? noise
      : '')
    if (progress < 1)
      scrambleFrame = requestAnimationFrame(tick)
  }
  scrambleFrame = requestAnimationFrame(tick)
}

// ── Cover plate: two layers; the new cover wipes in over the old one in
// 240ms, and four corner ticks frame the specimen. ──
const coverBack = ref<HTMLElement | null>(null)
const coverFront = ref<HTMLElement | null>(null)
let shownCover: string | null = null
/** Whether any cover has been painted: the first one has nothing to wipe over. */
let painted = false
const libraryUi = useLibraryUiStore()
/** The work whose veiled cover was shown by a click; any other work is hatched again. */
const shownRj = ref<string | null>(null)
const veiled = computed(() => libraryUi.veil && deferredWork.value?.nsfw === true && shownRj.value !== deferredWork.value.rj)
const { src: realCover, failed: coverFailed, isLocal: coverIsLocal } = useCoverSrc(() => {
  const detail = deferredWork.value
  return detail
    ? { rj: detail.rj, local: detail.local_cover, remote: detail.cover_url ?? detail.thumbnail_url }
    : null
})
const coverSrc = computed(() => (veiled.value ? null : realCover.value))
/** A CSS background never reports a load failure, so a local cover is proven loadable first. */
function probe(url: string, first: boolean): Promise<boolean> {
  return new Promise((resolve) => {
    const image = new Image()
    if (first)
      image.fetchPriority = 'high'
    image.onload = () => resolve(true)
    image.onerror = () => resolve(false)
    image.src = url
  })
}
// The plates are watched too: a body filled during setup is painted once
// they mount.
watch(
  () => [coverSrc.value, coverFront.value] as const,
  ([url]) => {
    if (url !== null && coverIsLocal(url) && url !== shownCover) {
      void probe(url, !painted).then((ok) => {
        if (url !== coverSrc.value)
          return
        if (ok)
          showCover(url)
        else coverFailed(url)
      })
      return
    }
    showCover(url)
  },
)
function showCover(url: string | null) {
  if (url === shownCover || !coverBack.value || !coverFront.value)
    return
  const first = !painted
  painted = true
  if (reducedMotion.value || switching.value || first) {
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
}
function paintCover(el: HTMLElement, url: string | null) {
  el.classList.toggle('hatch', url === null)
  el.style.backgroundImage = url === null
    ? ''
    : `url("${url}")`
}

/** `YYYY.MM.DD` from the catalog's `YYYY-MM-DD`. */
function dottedDate(value: string | null): string {
  return value
    ? value.replaceAll('-', '.')
    : '—'
}

const voice = computed(() =>
  deferredWork.value && deferredWork.value.voice_actors.length > 0
    ? deferredWork.value.voice_actors.map(actor => actor.name).join(' / ')
    : '—',
)

const tags = computed(() =>
  deferredWork.value && deferredWork.value.source_tags.length > 0
    ? deferredWork.value.source_tags.map(tag => tag.name).join(' / ')
    : '—',
)

const { playWork } = usePlayWork()

// PLAY needs the loaded work: it plays what the inspector shows from track 01.
const canPlay = computed(() => workNow.value !== null)
function playNowPlaying() {
  if (workNow.value)
    void playWork({ rj: workNow.value.rj, title: workNow.value.title ?? workNow.value.rj })
}
function openDetail() {
  if (props.rj !== null) {
    router.push({
      name: 'work-detail',
      params: { rj: props.rj },
    })
  }
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
      <div class="plate-frame" :class="{ veiled }">
        <div ref="coverBack" class="cover back" />
        <div ref="coverFront" class="cover front hatch" />
        <button v-if="veiled" class="btn btn-s reveal" type="button" @click="shownRj = deferredWork?.rj ?? null">SHOW</button>
        <i class="tick tl" /><i class="tick tr" /><i class="tick bl" /><i class="tick br" />
      </div>
    </div>

    <span class="insp-circle lbl">CIRCLE<b>{{ workNow?.circle ?? '—' }}</b></span>
    <h2 ref="titleEl" class="insp-title">{{ workNow?.title ?? '' }}</h2>

    <div class="strip"><span class="strip-name lbl">SPEC</span></div>
    <dl class="spec" :class="{ dim }">
      <div class="spec-row"><dt class="lbl">VOICE</dt><dd>{{ voice }}</dd></div>
      <div class="spec-row"><dt class="lbl">RELEASE</dt><dd>{{ dottedDate(deferredWork?.release_date ?? null) }}</dd></div>
      <div class="spec-row"><dt class="lbl">TAG</dt><dd class="clamp" :title="tags">{{ tags }}</dd></div>
    </dl>

    <WorkTracklist class="tracklist-slot" :rj="deferredWork?.rj ?? null" :title="deferredWork?.title ?? deferredWork?.rj ?? ''" :dim="dim" />

    <div class="insp-foot">
      <div class="insp-foot-inner">
        <button class="btn btn-primary" type="button" :disabled="!canPlay" @click="playNowPlaying">
          <svg viewBox="0 0 10 10" aria-hidden="true"><path :d="playIcon" /></svg>PLAY
        </button>
        <button v-if="!standalone" class="btn" type="button" :disabled="rj === null" @click="openDetail">
          OPEN DETAIL<svg viewBox="0 0 10 10" aria-hidden="true"><path :d="arrowIcon" /></svg>
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.insp {
  border-left: 1px solid var(--wire-mid);
  overflow-y: auto; min-height: 0;
  display: flex; flex-direction: column;
  padding: var(--sp-5) var(--sp-6) var(--sp-5);
  container-type: inline-size;
}

/* The specimen plate: a 4:3 cover never scaled above its native 560px. */
.plate {
  position: relative; width: 100%; max-width: var(--cover-native);
  margin: var(--sp-4) auto var(--sp-3);
}
/* On a short screen the plate is what gives way: it starts at its full-width
   4:3 height (0.75 = 1 / --cover-ratio) and shrinks until the rest fits, and
   the frame keeps 4:3 by narrowing, centred in the column. */
.insp > .plate { flex: 0 1 calc(min(100cqi, var(--cover-native)) * 0.75); min-height: 160px; }
.plate-frame { position: relative; height: 100%; max-width: 100%; aspect-ratio: var(--cover-ratio); margin: 0 auto; }
.cover {
  position: absolute; inset: 0; overflow: hidden;
  background-position: center;
  box-shadow: inset 0 0 0 1px var(--wire-fine);
}
.cover:not(.hatch) { background-size: cover; }
.veiled .cover::before { content: none; }
.reveal { position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%); z-index: 1; }
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
  /* Japanese never carries tracking. */
  letter-spacing: 0; min-height: var(--lh-l);
}

.spec { margin: var(--sp-1) 0 var(--sp-4); }
/* One field per row: a fixed label column, and the value wraps rather than
   widening the panel — a cast or tag list is as long as the work says. */
.spec-row {
  display: grid; grid-template-columns: 64px minmax(0, 1fr); align-items: baseline; gap: var(--sp-2);
  min-height: 28px; padding: 8px 0 6px; border-bottom: 1px solid var(--wire-fine);
}
.spec-row dd { font-size: var(--fs-s); line-height: var(--lh-s); font-weight: 500; overflow-wrap: anywhere; }
/* A tag list is open-ended: two lines, the rest on hover. */
.spec-row dd.clamp { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; }
.spec { transition: opacity 80ms linear; }
.dim { opacity: .3; }

.insp > * { flex-shrink: 0; }
/* The tracklist takes the height the 1440x900 frame leaves and scrolls inside it;
   its wrapper dissolves so a short panel scrolls instead of the list spilling. */
.tracklist-slot { display: contents; }
.tracklist-slot :deep(.strip) { flex-shrink: 0; }
.tracklist-slot :deep(.tracks) { flex: 1 1 0; min-height: 84px; }

.insp-foot { margin-top: auto; padding-top: var(--sp-4); }
.insp-foot-inner {
  border-top: 2px solid var(--wire-strong);
  padding-top: var(--sp-4);
  display: flex; justify-content: space-between; align-items: center; gap: var(--sp-3);
}
.insp-foot .btn:disabled { cursor: not-allowed; opacity: .5; }
</style>
