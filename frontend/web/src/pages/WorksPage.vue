<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { canonicalRj } from '@/app/route-params'
import { useMediaQuery, WALL_MOBILE_QUERY } from '@/composables/use-media-query'
import { useTheme } from '@/composables/use-theme'
import type { ListWorksParams, WorkFacet, WorksSort } from '@/lib/api/types'
import LibrarySwitcher from '@/features/works/LibrarySwitcher.vue'
import PlayerBar from '@/features/works/PlayerBar.vue'
import RailSelect, { type RailOption } from '@/features/works/RailSelect.vue'
import WorkInspector from '@/features/works/WorkInspector.vue'
import WorkWall from '@/features/works/WorkWall.vue'
import { useWorkFacets, useWorksList } from '@/queries/works'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'

// The work wall page (ADR 0012 §4): an in-page header, a filter rail, the
// bench of wall and inspector, and the player baseline — four rows inside the
// AppShell's main. The selected work lives in the `?rj=` identity; filters,
// the order and the FIND box are the page's transient state, never the address's.

const route = useRoute()
const router = useRouter()
const { theme } = useTheme()
const libraryUi = useLibraryUiStore()
const player = usePlayerStore()
const isMobile = useMediaQuery(WALL_MOBILE_QUERY)

// ── Filter rail: voice actor, circle and age rating, each one listing
// parameter; the order is its own select; FIND is q. '' = no constraint. ──
const voiceActor = ref('')
const circle = ref('')
const age = ref('')
const sort = ref<WorksSort>('release_date')

const libraryScope = computed(() => libraryUi.activeLibraryId)
const { facets: voiceActorFacets } = useWorkFacets('voice_actor', libraryScope)
const { facets: circleFacets } = useWorkFacets('circle', libraryScope)
// An entity without a source id is kept per work (ADR 0011 §6), so one name
// can arrive as several ids. The rail offers the name once and asks for all of
// its ids: the listing matches any id of a comma-separated list.
function facetOptions(facets: WorkFacet[]): RailOption[] {
  const byName = new Map<string, { ids: string[]; count: number }>()
  for (const facet of facets) {
    const entry = byName.get(facet.name) ?? { ids: [], count: 0 }
    entry.ids.push(facet.id)
    entry.count += facet.work_count
    byName.set(facet.name, entry)
  }
  return [
    { value: '', label: 'ALL' },
    ...[...byName].map(([name, entry]) => ({ value: entry.ids.join(','), label: name, count: entry.count })),
  ]
}
const voiceActorOptions = computed(() => facetOptions(voiceActorFacets.value))
const circleOptions = computed(() => facetOptions(circleFacets.value))

// Unknown is its own answer, not a synonym for 全年齢 (docs/api.md `nsfw`).
const AGE_OPTIONS: RailOption[] = [
  { value: '', label: 'ALL' },
  { value: 'safe', label: '全年齢' },
  { value: 'r18', label: 'R18' },
  { value: 'unknown', label: 'UNKNOWN' },
]
const AGE_PARAM: Record<string, ListWorksParams['nsfw']> = { safe: false, r18: true, unknown: 'unknown' }

// Every order runs newest / highest first; unknown dates and unrated works
// trail in either direction.
const SORT_OPTIONS: RailOption[] = [
  { value: 'release_date', label: 'RELEASE' },
  { value: 'collected_at', label: 'ADDED' },
  { value: 'rating', label: 'RATING' },
]

const findText = ref('')
const findQuery = ref('')
let findTimer: number | null = null
watch(findText, (value) => {
  if (findTimer !== null) window.clearTimeout(findTimer)
  findTimer = window.setTimeout(() => {
    findQuery.value = value.trim()
  }, 200)
})
onBeforeUnmount(() => {
  if (findTimer !== null) window.clearTimeout(findTimer)
})

const filters = computed<ListWorksParams>(() => ({
  ...(libraryUi.activeLibraryId ? { library: libraryUi.activeLibraryId } : {}),
  ...(voiceActor.value ? { voice_actor: voiceActor.value, voice_actor_match: 'any' as const } : {}),
  ...(circle.value ? { circle: circle.value } : {}),
  ...(age.value ? { nsfw: AGE_PARAM[age.value] } : {}),
  ...(findQuery.value ? { q: findQuery.value } : {}),
  sort: sort.value,
  direction: 'desc',
}))

const { works, query } = useWorksList(filters)
// No total exists in the listing (keyset pages only): the header count is what
// has loaded so far and grows while you scroll.
const loadedCount = computed(() => works.value.length)

// ── Selection: restored from ?rj=, kept valid against the current listing,
// and written back so a refresh keeps the inspected work. ──
const restoredRj = typeof route.query.rj === 'string' ? canonicalRj(route.query.rj) : null
const selectedRj = ref<string | null>(restoredRj)
// The address may name a work beyond the pages loaded so far: it stays
// inspected until the user changes the listing under it.
let keepRestored = restoredRj !== null
watch([voiceActor, circle, age, sort, findQuery], () => {
  keepRestored = false
})
// A voice actor or circle belongs to the library it was picked in. The first
// resolution of the active library (null → the stored or first one) is not a
// switch, so it leaves the address's work alone.
watch(libraryScope, (_now, before) => {
  voiceActor.value = ''
  circle.value = ''
  if (before !== null) keepRestored = false
})

watch(works, (list) => {
  // The previous filter's wall, shown while the new listing loads.
  if (query.isPlaceholderData.value) return
  if (selectedRj.value !== null && list.some((work) => work.rj === selectedRj.value)) return
  if (keepRestored) return
  selectedRj.value = list.length > 0 ? list[0].rj : null
})

watch(selectedRj, (rj) => {
  // Replace, not push: inspecting a neighbouring work is one state, not a
  // page in history.
  router.replace({ query: { ...route.query, rj: rj ?? undefined } })
})

// ── Keyboard: the wall's dispatch, with the same guards as the prototype. ──
const wall = ref<InstanceType<typeof WorkWall> | null>(null)

function playSelected(): void {
  const work = works.value.find((candidate) => candidate.rj === selectedRj.value)
  if (work) player.play({ rj: work.rj, title: work.title ?? work.rj })
}

function loadMore(): void {
  // One page at a time: a second request would cancel the one in flight, and
  // the placeholder wall's cursor belongs to the previous filter.
  if (query.hasNextPage.value && !query.isFetchingNextPage.value && !query.isPlaceholderData.value) {
    // ponytail: fetch errors surface through the query state the wall already
    // renders; per-error UI here when a listing failure needs its own copy.
    query.fetchNextPage().catch(() => {})
  }
}

function openWork(rj: string): void {
  router.push({ name: 'work-detail', params: { rj } })
}

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
  // defineExpose unwraps the computed, so the instance carries the number.
  const cols = wall.value?.columns ?? 1
  const moves: Record<string, number> = {
    j: 1,
    k: -1,
    ArrowRight: 1,
    ArrowLeft: -1,
    ArrowDown: cols,
    ArrowUp: -cols,
  }
  if (event.key in moves) {
    event.preventDefault()
    wall.value?.move(moves[event.key])
    return
  }
  if (event.key === 'p') {
    playSelected()
    return
  }
  if (activates(event.target)) return
  if (event.key === 'Enter' && selectedRj.value !== null) {
    openWork(selectedRj.value)
  } else if (event.key === ' ') {
    // Space only pauses and resumes: browsing never interrupts playback.
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

const themeLabel = computed(() => (theme.value === 'light' ? 'DARK' : 'LIGHT'))
function toggleTheme(): void {
  theme.value = theme.value === 'light' ? 'dark' : 'light'
}
</script>

<template>
  <section class="works substrate" aria-label="作品库">
    <header class="header">
      <div class="header-left">
        <LibrarySwitcher />
      </div>
      <div class="header-meta">
        <span class="lbl count">WORKS<b>{{ loadedCount }}</b></span>
        <button class="btn btn-s" type="button" title="Switch theme" @click="toggleTheme">
          {{ themeLabel }}
        </button>
      </div>
    </header>

    <div class="rail">
      <div class="filters" role="group" aria-label="Filter">
        <RailSelect v-model="voiceActor" label="VOICE" :options="voiceActorOptions" marks-active searchable />
        <RailSelect v-model="circle" label="CIRCLE" :options="circleOptions" marks-active searchable />
        <RailSelect v-model="age" label="AGE" :options="AGE_OPTIONS" marks-active />
      </div>
      <div class="rail-right">
        <RailSelect v-model="sort" label="SORT" :options="SORT_OPTIONS" />
        <label class="find">
          <span class="lbl">FIND</span>
          <input v-model="findText" type="text" placeholder="RJ / TITLE" />
        </label>
      </div>
    </div>

    <main class="bench">
      <WorkWall ref="wall" :works="works" :selected-rj="selectedRj" :playing-rj="player.rj"
        @select="selectedRj = $event" @open="openWork" @load-more="loadMore" />
      <!-- A phone has no inspector: a tap routes to the detail page instead. -->
      <WorkInspector v-if="!isMobile" :rj="selectedRj" />
    </main>

    <PlayerBar />
  </section>
</template>

<style scoped>
/* App rows: header 48, filter rail 40, work area 1fr, bottom bar 28. The two
   outer edges carry the only double bar (1px fine + 2px strong); everything
   inside uses single 1px lines. */
.works {
  display: grid; grid-template-rows: 48px 40px minmax(0, 1fr) 28px;
  width: 100%; height: 100%;
  font-variant-numeric: tabular-nums;
}

.header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0 var(--sp-6); position: relative;
}
.header::after {
  content: ''; position: absolute; left: var(--sp-6); right: var(--sp-6); bottom: 0; height: 4px;
  border-top: 1px solid var(--wire-fine); border-bottom: 2px solid var(--wire-strong);
}
.header-meta { display: flex; align-items: center; gap: var(--sp-6); }
.header-meta b { color: var(--fg-base); font-weight: 600; letter-spacing: .08em; }

.rail {
  display: flex; align-items: center; justify-content: space-between; gap: var(--sp-4);
  padding: 0 var(--sp-6); position: relative;
}
.rail::after {
  content: ''; position: absolute; left: var(--sp-6); right: var(--sp-6); bottom: 0;
  border-bottom: 1px solid var(--wire-fine);
}
.filters, .rail-right { display: flex; align-items: center; gap: var(--sp-1); min-width: 0; }
.rail-right { gap: var(--sp-4); flex-shrink: 0; }

.find { display: flex; align-items: center; gap: var(--sp-2); flex-shrink: 0; }
.find input {
  height: 24px; width: 184px; padding: 0 var(--sp-2);
  border: 1px solid var(--wire-control); background: transparent; color: var(--fg-base);
  font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .08em;
}
.find input::placeholder { color: var(--fg-meta); }
.find input:focus { border-color: var(--wire-strong); outline: none; }

.bench {
  display: grid; grid-template-columns: minmax(0, 1fr) clamp(400px, 34vw, 608px);
  min-height: 0;
}

.header-left { min-width: 0; }

@media (max-width: 768px) {
  .bench { grid-template-columns: 1fr; }
  .header, .rail { padding: 0 var(--sp-4); }
  .header::after, .rail::after { left: var(--sp-4); right: var(--sp-4); }
  .find { display: none; }
  .header-meta .count { display: none; }
}
</style>
