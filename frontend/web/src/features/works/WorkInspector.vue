<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { REDUCED_MOTION_QUERY, useMediaQuery } from '@/composables/use-media-query'
import type { WorkDetail } from '@/lib/api/types'
import { deriveTracks, fmtLong, fmtT, playableCopy } from '@/features/work-detail/derive'
import { usePlayWork } from '@/features/works/use-play-work'
import { useCoverSrc } from '@/features/works/use-cover-src'
import { useWorkContent, useWorkDetail } from '@/queries/works'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'

// The inspector: the detail of the selected work, rendered to fit 1440x900
// without scrolling. While cruising the wall the text follows
// immediately, but the cover/SPEC/TRACKLIST dim to 30% and are swapped only
// after a 70ms pause, so fast browsing merges updates instead of flickering.

const props = defineProps<{
  rj: string | null
  /** On the detail page: no wall to return to, so no OPEN DETAIL and no wall keys. */
  standalone?: boolean
  /**
   * Show (and play) only the tracks that exist in this codec, e.g. 'WAV'. Null,
   * or a codec the work does not ship, leaves the merged list. The entry point
   * is the settings page's, later.
   */
  onlyFormat?: string | null
}>()

const router = useRouter()
const player = usePlayerStore()
const { work } = useWorkDetail(() => props.rj)

/** The detail of the work actually being displayed, or null while loading. */
const workNow = computed(() => (work.value && props.rj !== null && work.value.rj === props.rj ? work.value : null))

// ── Deferred body: cover + SPEC + TRACKLIST swap after a 70ms pause. ──
// Seeded from the cache: a remount (the wall leaving and re-entering the
// desktop layout) finds the work already loaded, and no change would fill it.
const deferredWork = ref<WorkDetail | null>(workNow.value)
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
const libraryUi = useLibraryUiStore()
/** The work whose veiled cover was shown by a click; any other work is hatched again. */
const shownRj = ref<string | null>(null)
const veiled = computed(() => libraryUi.veil && deferredWork.value?.nsfw === true && shownRj.value !== deferredWork.value.rj)
const { src: realCover, failed: coverFailed, isLocal: coverIsLocal } = useCoverSrc(() => {
  const detail = deferredWork.value
  return detail ? { rj: detail.rj, local: detail.local_cover, remote: detail.cover_url ?? detail.thumbnail_url } : null
})
const coverSrc = computed(() => (veiled.value ? null : realCover.value))
/** A CSS background never reports a load failure, so a local cover is proven loadable first. */
function probe(url: string): Promise<boolean> {
  return new Promise((resolve) => {
    const image = new Image()
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
      void probe(url).then((ok) => {
        if (url !== coverSrc.value) return
        if (ok) showCover(url)
        else coverFailed(url)
      })
      return
    }
    showCover(url)
  },
)
function showCover(url: string | null) {
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
}
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

const tags = computed(() =>
  deferredWork.value && deferredWork.value.source_tags.length > 0
    ? deferredWork.value.source_tags.map((tag) => tag.name).join(' / ')
    : '—',
)

// ── Tracklist: the detail page's track list (deriveTracks over the first
// online copy); a row plays the work from that track. The manifest follows the deferred work, so
// cruising the wall fetches only the works it settles on. The list takes the
// height the 1440x900 frame leaves and scrolls inside it. ──
const { content, query: contentQuery } = useWorkContent(() => deferredWork.value?.rj ?? null)
const allTracks = computed(() => {
  const tree = playableCopy(content.value)?.tree
  return tree ? deriveTracks(tree) : []
})
const codec = computed(() =>
  props.onlyFormat && allTracks.value.some((track) => track.files[props.onlyFormat!]) ? props.onlyFormat : null,
)
const tracks = computed(() =>
  codec.value ? allTracks.value.filter((track) => track.files[codec.value!]) : allTracks.value,
)
// A variant header only helps when the work has more than one variant.
const multiVariant = computed(() => new Set(tracks.value.map((track) => track.variant)).size > 1)
const showHeader = (t: number): boolean =>
  multiVariant.value && tracks.value[t].variant !== tracks.value[t - 1]?.variant
const tracksSummary = computed(() => {
  if (tracks.value.length === 0) return '—'
  const total = tracks.value.reduce((sum, track) => sum + (track.duration ?? 0), 0)
  return total > 0 ? `${tracks.value.length} · ${fmtLong(total)}` : String(tracks.value.length)
})

const { playFrom, playWork } = usePlayWork()
const isPlaying = (t: number): boolean =>
  player.rj === deferredWork.value?.rj && player.trackKey === tracks.value[t]?.key
function playTrack(t: number) {
  const shown = deferredWork.value
  if (shown) {
    const index = allTracks.value.indexOf(tracks.value[t])
    playFrom({ rj: shown.rj, title: shown.title ?? shown.rj }, content.value, index, codec.value ?? '')
  }
}

// PLAY needs the loaded work: it plays what the inspector shows from track 01.
const canPlay = computed(() => workNow.value !== null)
function playNowPlaying() {
  if (workNow.value) void playWork({ rj: workNow.value.rj, title: workNow.value.title ?? workNow.value.rj }, codec.value ?? '')
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
      <div class="plate-frame" :class="{ veiled }">
        <div ref="coverBack" class="cover back"></div>
        <div ref="coverFront" class="cover front hatch"></div>
        <button v-if="veiled" class="btn btn-s reveal" type="button" @click="shownRj = deferredWork?.rj ?? null">SHOW</button>
        <i class="tick tl"></i><i class="tick tr"></i><i class="tick bl"></i><i class="tick br"></i>
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

    <div class="strip">
      <span class="strip-name lbl">TRACKLIST</span>
      <span class="lbl">{{ tracksSummary }}</span>
    </div>
    <div class="tracks" :class="{ dim }" lang="ja">
      <template v-for="(track, t) in tracks" :key="track.key">
        <div v-if="showHeader(t)" class="var lbl">{{ track.variant || '/' }}</div>
        <button class="trk" :class="{ now: isPlaying(t) }"
          type="button" :aria-current="isPlaying(t) || undefined" @click="playTrack(t)">
          <span class="no lbl">{{ String(t + 1).padStart(2, '0') }}</span>
          <span class="nm">{{ track.stem }}</span>
          <span class="dur">{{ track.duration === null ? '—' : fmtT(track.duration) }}</span>
        </button>
      </template>
      <p v-if="tracks.length === 0 && !contentQuery.isPending.value" class="no-tracks lbl">NO TRACK DATA</p>
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
  background-size: cover; background-position: center;
  box-shadow: inset 0 0 0 1px var(--wire-fine);
}
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
.spec, .tracks { transition: opacity 80ms linear; }
.dim { opacity: .3; }

.insp > * { flex-shrink: 0; }
.tracks {
  margin-top: var(--sp-2); font-family: var(--font-ja);
  flex: 1 1 0; min-height: 84px; overflow-y: auto;
}
.trk {
  display: grid; grid-template-columns: 24px minmax(0, 1fr) 44px; align-items: center; gap: var(--sp-3);
  width: 100%; height: 28px; padding: 0 var(--sp-2); border-bottom: 1px solid var(--wire-fine);
  text-align: left; color: var(--fg-base); font-family: inherit; cursor: pointer;
}
.var { padding: var(--sp-2) var(--sp-2) var(--sp-1); font-size: var(--fs-xs); color: var(--fg-meta); border-bottom: 1px solid var(--wire-fine); }
.trk:hover { background: var(--bg-subtle); }
.trk:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: -1px; }
.trk .lbl { letter-spacing: .04em; }
.trk .nm { font-size: var(--fs-s); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.trk .dur { text-align: right; font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted); }
/* The playing track: inverted with the bay's ■ marker ( --now). */
.trk.now { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.trk.now .lbl, .trk.now .dur { color: var(--invert-fg); }
.trk .no { position: relative; }
.trk.now .no::after { content: ''; position: absolute; left: 16px; top: 3px; width: 6px; height: 6px; background: var(--now); }
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
