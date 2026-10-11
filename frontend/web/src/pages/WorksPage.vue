<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { canonicalRj } from '@/app/route-params'
import { useMediaQuery, WALL_MOBILE_QUERY } from '@/composables/use-media-query'
import { useApiClient } from '@/lib/api/client'
import type { ListWorksParams, WorkFacet, WorksScope, WorksSort } from '@/lib/api/types'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import LibrarySwitcher from '@/features/works/LibrarySwitcher.vue'
import TagIndex, { type TagSelection } from '@/features/works/TagIndex.vue'
import { usePlayWork } from '@/features/works/use-play-work'
import { PAGE_SIZES, useWallQuery } from '@/features/works/use-wall-query'
import { useRecordWallReturn, useWallRestore, type WallHandle } from '@/features/works/use-wall-restore'
import PlayerBar from '@/features/works/PlayerBar.vue'
import RailSelect, { type RailOption } from '@/features/works/RailSelect.vue'
import WorkInspector from '@/features/works/WorkInspector.vue'
import WorkPager from '@/features/works/WorkPager.vue'
import WorkWall from '@/features/works/WorkWall.vue'
import { useLibraryList } from '@/queries/libraries'
import { useWorkFacets, useWorksList, worksListQueryOptions } from '@/queries/works'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'

// The work wall page: an in-page header, a filter rail, the
// bench of one numbered page of works with its pager and the inspector, and
// the player baseline — four rows inside the AppShell's main.
//
// State ownership: what the wall shows — library, filters, FIND, order, page,
// page size and the selected work — is the address's, through useWallQuery and
// its explicit actions. Where the wall was when a work was opened belongs to
// that history entry (use-wall-restore). The page-size preference only seeds a
// fresh visit. Typing in FIND, the tag index's staged picks and open menus stay
// in their components.

const router = useRouter()
const api = useApiClient()
const queryClient = useQueryClient()
const libraryUi = useLibraryUiStore()
const player = usePlayerStore()
const isMobile = useMediaQuery(WALL_MOBILE_QUERY)
const wall = ref<InstanceType<typeof WorkWall> | null>(null)
const wq = useWallQuery()

// ── Library: the address names it and the store mirrors it, for the switcher
// and the other pages. The store differing from the address can only be the
// switcher's pick — restoring an address sets them equal first — so only that
// drops the previous library's picks. ──
const { librariesData } = useLibraryList()
watch(wq.library, (library) => {
  if (library === 'all') libraryUi.showAllLibraries()
  else if (library) libraryUi.setActiveLibrary(library)
}, { immediate: true })
const storeLibrary = computed(() => (libraryUi.allLibraries ? 'all' : libraryUi.activeLibraryId))
watch(storeLibrary, (library) => {
  if (library === null || library === wq.library.value) return
  if (wq.library.value === null) void wq.fillLibrary(library)
  else void wq.setLibrary(library)
}, { immediate: true })
// `all` is the aggregate wall: every library's id, so the listing, the facets
// and FIND share one copy-scoped filter and a work held twice is listed once.
const libraryScope = computed(() =>
  libraryUi.allLibraries
    ? (librariesData.value.map((library) => library.id).join(',') || null)
    : libraryUi.activeLibraryId,
)

// ── Filter rail: each select reads the address and writes through an action.
// '' = no constraint. ──
const voiceActor = computed({ get: () => wq.voice.value, set: (value) => void wq.setFilters({ voice: value }) })
const circle = computed({ get: () => wq.circle.value, set: (value) => void wq.setFilters({ circle: value }) })
const age = computed({ get: () => wq.age.value, set: (value) => void wq.setFilters({ age: value }) })
const language = computed({ get: () => wq.language.value, set: (value) => void wq.setFilters({ language: value }) })
const sort = computed<WorksSort>({ get: () => wq.sort.value, set: (value) => void wq.setSort(value) })
const size = computed({ get: () => String(wq.size.value), set: (value) => void wq.setSize(Number(value)) })
const tags = computed<TagSelection>({
  get: () => ({
    groups: wq.tag.value ? wq.tag.value.split(',').map((group) => group.split('|')) : [],
    mode: wq.mode.value,
  }),
  set: (value) => void wq.setFilters({
    tag: value.groups.map((group) => group.join('|')).join(','),
    mode: value.groups.length > 0 && value.mode === 'any' ? 'any' : '',
  }),
})

// Age categories preserve the upstream distinction between 全年龄, R15 and R18.
const AGE_OPTIONS: RailOption[] = [
  { value: '', label: 'ALL' },
  { value: 'safe', label: '全年龄' },
  { value: 'r15', label: 'R15' },
  { value: 'r18', label: 'R18' },
]
const LANGUAGE_OPTIONS: RailOption[] = [
  { value: '', label: 'ALL' },
  { value: 'CHI_HANS', label: '简中' },
  { value: 'CHI_HANT', label: '繁中' },
  { value: 'JPN', label: '日语' },
  { value: 'NM', label: '无语言限制' },
]
const AGE_PARAM: Record<string, ListWorksParams['age_category']> = { safe: 1, r15: 2, r18: 3 }

// Every order runs newest / highest first; unknown dates and unrated works
// trail in either direction.
const SORT_OPTIONS: RailOption[] = [
  { value: 'rj', label: '作品编号' },
  { value: 'release_date', label: '发售时间' },
  { value: 'collected_at', label: '添加时间' },
  { value: 'rating', label: '个人评分' },
]
const SIZE_OPTIONS: RailOption[] = PAGE_SIZES.map((value) => ({ value: String(value), label: String(value) }))

// The set of works on the wall; the order and the page only arrange it.
const scope = computed<WorksScope>(() => ({
  ...(libraryScope.value ? { library: libraryScope.value } : {}),
  ...(wq.voice.value ? { voice_actor: wq.voice.value, voice_actor_match: 'any' as const } : {}),
  ...(wq.circle.value ? { circle: wq.circle.value } : {}),
  ...(wq.language.value ? { language: wq.language.value } : {}),
  ...(wq.age.value ? { age_category: AGE_PARAM[wq.age.value] } : {}),
  ...(wq.tag.value ? { tag: wq.tag.value, tag_match: wq.mode.value } : {}),
  ...(wq.q.value ? { q: wq.q.value } : {}),
}))
const filters = computed<ListWorksParams>(() => ({
  ...scope.value,
  sort: wq.sort.value,
  direction: 'desc',
  page: wq.page.value,
  limit: wq.size.value,
}))
// An address past the length limit is not listed: answering it with a wider
// listing would misstate what is shown.
const listable = computed(() => !wq.tooLong.value)

// Each rail offers what the wall's other filters can still combine with, and
// leaves its own pick out so the alternatives to it stay on offer.
function scopeWithout(...keys: (keyof WorksScope)[]) {
  return computed(() => {
    const rest = { ...scope.value }
    for (const key of keys) delete rest[key]
    return rest
  })
}
const libraryOnly = computed<WorksScope>(() => (libraryScope.value ? { library: libraryScope.value } : {}))
/** Whether a pick has no entity left in its narrowed list, so its name must be looked up. */
const unnamed = (picked: string, facets: WorkFacet[]) => {
  if (picked === '') return false
  const ids = picked.split(',')
  return !facets.some((facet) => ids.includes(facet.id))
}
const { facets: voiceActorFacets } = useWorkFacets('voice_actor', scopeWithout('voice_actor', 'voice_actor_match'), listable)
const { facets: circleFacets } = useWorkFacets('circle', scopeWithout('circle'), listable)
const { facets: voiceActorNames } = useWorkFacets('voice_actor', libraryOnly,
  () => unnamed(wq.voice.value, voiceActorFacets.value))
const { facets: circleNames } = useWorkFacets('circle', libraryOnly, () => unnamed(wq.circle.value, circleFacets.value))
const tagScope = scopeWithout('tag', 'tag_match')

// An entity without a source id is kept per work (ADR 0009), so one name
// can arrive as several ids. The rail offers the name once and asks for all of
// its ids: the listing matches any id of a comma-separated list. A pick the
// other filters have narrowed away stays on offer at 0 — named from the
// library's whole list, or as unknown when even that lacks it — so the tab
// still says what the wall is filtered by and ALL still clears it.
function facetOptions(facets: WorkFacet[], picked: string, names: WorkFacet[]): RailOption[] {
  const byName = new Map<string, { ids: string[]; count: number }>()
  for (const facet of facets) {
    const entry = byName.get(facet.name) ?? { ids: [], count: 0 }
    entry.ids.push(facet.id)
    entry.count += facet.work_count
    byName.set(facet.name, entry)
  }
  const options = [...byName].map(([name, entry]) => ({ value: entry.ids.join(','), label: name, count: entry.count }))
  if (picked !== '' && !options.some((option) => option.value === picked)) {
    const ids = picked.split(',')
    const name = [...facets, ...names].find((facet) => ids.includes(facet.id))?.name
    options.push({ value: picked, label: name ?? `未知 ×${ids.length}`, count: 0 })
  }
  return [{ value: '', label: 'ALL' }, ...options]
}
const voiceActorOptions = computed(() => facetOptions(voiceActorFacets.value, wq.voice.value, voiceActorNames.value))
const circleOptions = computed(() => facetOptions(circleFacets.value, wq.circle.value, circleNames.value))

// ── The page of works and the corrections it implies. Only settled data for
// the current address may correct anything: a placeholder is the previous
// address's page, and its total is not this one's. ──
const { works, total, query } = useWorksList(filters, listable)
const settled = computed(() => query.isSuccess.value && !query.isPlaceholderData.value)
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / wq.size.value)))
const selectedRj = computed(() => wq.rj.value)
/** Which end of the next page a page turn lands on: a move back off the first work lands on the last. */
let landOn: 'first' | 'last' = 'first'

watch([settled, () => query.data.value, wq.rj], () => {
  const data = query.data.value
  if (!settled.value || !data) return
  const last = Math.max(1, Math.ceil(data.total / wq.size.value))
  if (wq.page.value > last) {
    void wq.setPage(last)
    return
  }
  if (data.works.length === 0) {
    if (wq.rj.value !== null) void wq.select(null)
    return
  }
  // The selection stays on the page it is shown on. A work that is not here —
  // deleted, moved off the page by an order or a concurrent change, or named
  // by a link to another page — falls back to this page, without a lookup.
  if (wq.rj.value !== null && data.works.some((work) => work.rj === wq.rj.value)) return
  const target = landOn === 'last' ? data.works[data.works.length - 1] : data.works[0]
  landOn = 'first'
  void wq.select(target.rj)
}, { immediate: true })

function turnPage(to: number, land: 'first' | 'last' = 'first', claimFocus = false): void {
  if (to < 1 || to > pageCount.value || to === wq.page.value) return
  landOn = land
  if (claimFocus) wall.value?.claimFocus()
  void wq.setPage(to)
}

// ── FIND: the box is a draft; its debounced text is the address's `q`. An
// address change replaces the draft and cancels a pending write. ──
const findText = ref(wq.q.value)
let findTimer: number | null = null
let findSubmission = 0
function cancelFind(): void {
  findSubmission++
  if (findTimer !== null) window.clearTimeout(findTimer)
  findTimer = null
}
watch(findText, (value) => {
  cancelFind()
  if (value.trim() === wq.q.value) return
  findTimer = window.setTimeout(() => {
    findTimer = null
    void wq.setFilters({ q: value.trim() })
  }, 200)
})
watch(wq.q, (q) => {
  if (q === findText.value.trim()) return
  cancelFind()
  findText.value = q
})
onBeforeUnmount(cancelFind)

async function submitFind(event: KeyboardEvent): Promise<void> {
  if (event.isComposing || event.keyCode === 229) return
  const text = findText.value.trim()
  const code = canonicalRj(text)
  if (!code) return
  event.preventDefault()
  cancelFind()
  const submission = findSubmission
  // The address first, then the listing of exactly that address.
  await wq.setFilters({ q: text })
  let data
  try {
    data = await queryClient.fetchQuery(worksListQueryOptions(api, filters.value))
  } catch {
    // ponytail: the failed listing surfaces through the wall's own query.
    return
  }
  if (submission !== findSubmission) return
  const collected = data.works.filter((candidate) => candidate.copy_count > 0)
  const exact = collected.find((candidate) => candidate.rj === code)
  if (exact) await openWork(exact.rj)
  else if (data.total === data.works.length && collected.length === 1) await openWork(collected[0].rj)
}

// ── Opening a work: every entry (click, tap, Enter, FIND) commits the work to
// the address, records where the wall was on this history entry, and only then
// leaves — so BACK returns to exactly this wall. ──
const recordReturn = useRecordWallReturn()
async function openWork(rj: string): Promise<void> {
  await wq.select(rj)
  recordReturn({ scrollTop: wall.value?.scroller?.scrollTop ?? 0, focusRj: rj })
  await router.push({ name: 'work-detail', params: { rj } })
}

useWallRestore({
  ready: () => settled.value,
  wall: wall as Ref<WallHandle | null>,
  selectedRj: () => wq.rj.value,
})

// ── Keyboard: the wall's dispatch, with the same guards as the prototype. ──
const { playWork } = usePlayWork()
function playSelected(): void {
  const work = works.value.find((candidate) => candidate.rj === selectedRj.value)
  if (work) void playWork({ rj: work.rj, title: work.title ?? work.rj })
}

/** A text field or an open menu keeps every key it receives. */
function ownsKeys(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.closest('[data-own-keys]') !== null ||
    // A dialog (the tag index) is portalled outside the page: its keys are its own.
    target.closest('[role="dialog"]') !== null ||
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
  if (event.key === 'PageDown' || event.key === 'PageUp') {
    event.preventDefault()
    turnPage(wq.page.value + (event.key === 'PageDown' ? 1 : -1), 'first', true)
    return
  }
  if (event.key === 'p') {
    playSelected()
    return
  }
  if (activates(event.target)) return
  if (event.key === 'Enter' && selectedRj.value !== null) {
    void openWork(selectedRj.value)
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
</script>

<template>
  <section class="works substrate" aria-label="作品库">
    <header class="header">
      <div class="header-left">
        <GlobalMenu />
        <LibrarySwitcher />
      </div>
      <div class="header-meta">
        <button class="btn btn-s" type="button" :aria-pressed="libraryUi.veil"
          title="观看者选项：R18 作品的封面与插画先以排线遮盖"
          @click="libraryUi.toggleVeil()">R18<span class="veil-state">{{ libraryUi.veil ? ' VEIL ON' : ' VEIL OFF' }}</span></button>
        <span class="lbl count">WORKS<b>{{ total }}</b></span>
      </div>
    </header>

    <div class="rail">
      <div class="filters" role="group" aria-label="Filter">
        <RailSelect v-model="voiceActor" label="VOICE" :options="voiceActorOptions" marks-active searchable />
        <RailSelect v-model="circle" label="CIRCLE" :options="circleOptions" marks-active searchable />
        <RailSelect v-model="age" label="AGE" :options="AGE_OPTIONS" marks-active />
        <RailSelect v-model="language" label="LANGUAGE" :options="LANGUAGE_OPTIONS" marks-active />
        <TagIndex v-model="tags" :scope="tagScope" />
        <span v-if="wq.overflow.value" class="lbl notice" role="status">筛选条件过多，未应用</span>
      </div>
      <div class="rail-right">
        <RailSelect v-model="sort" label="SORT" :options="SORT_OPTIONS" />
        <RailSelect v-model="size" label="SIZE" :options="SIZE_OPTIONS" />
        <label class="find">
          <span class="lbl">FIND</span>
          <input v-model="findText" type="text" placeholder="RJ / TITLE" @keydown.enter="submitFind" />
        </label>
      </div>
    </div>

    <main class="bench">
      <div class="wall-col">
        <div class="wall-note">
          <p v-if="!listable" class="lbl notice" role="alert">地址中的筛选条件过长，未列出作品；请在筛选栏清除部分条件。</p>
          <p v-else-if="settled && total === 0" class="lbl notice">没有作品</p>
        </div>
        <WorkWall ref="wall" :works="works" :selected-rj="selectedRj" :playing-rj="player.rj"
          @select="wq.select($event)" @open="openWork"
          @edge="(direction) => turnPage(wq.page.value + direction, direction < 0 ? 'last' : 'first', true)" />
        <WorkPager :page="wq.page.value" :page-count="pageCount" @go="turnPage" />
      </div>
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
  display: grid; grid-template-rows: 48px 40px minmax(0, 1fr) auto; grid-template-columns: minmax(0, 1fr);
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
/* The wall scrolls; the pager stays under it. */
.wall-col { display: grid; grid-template-rows: auto minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
.notice { padding: var(--sp-3) var(--sp-6) 0; letter-spacing: .04em; text-transform: none; }
.rail .notice { padding: 0 var(--sp-2); }

.header-left { display: flex; align-items: center; gap: var(--sp-2); min-width: 0; }

@media (max-width: 768px) {
  .bench { grid-template-columns: 1fr; }
  .header, .rail { padding: 0 var(--sp-4); }
  .header::after, .rail::after { left: var(--sp-4); right: var(--sp-4); }
  .find { display: none; }
  .header-meta .count, .veil-state { display: none; }
}
</style>
