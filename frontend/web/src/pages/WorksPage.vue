<script setup lang="ts">
import type { Ref } from 'vue'
import type { RailOption } from '@/features/works/RailSelect.vue'
import type { TagSelection } from '@/features/works/TagIndex.vue'
import type { WallHandle } from '@/features/works/use-wall-restore'
import type { ListWorksParams, WorkFacet, WorksScope, WorksSort } from '@/lib/api/types'
import { useQueryClient } from '@tanstack/vue-query'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import { canonicalRj } from '@/app/route-params'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import { useKeymap } from '@/composables/use-keymap'
import { useMediaQuery, WALL_MOBILE_QUERY } from '@/composables/use-media-query'
import { useToast } from '@/composables/use-toast'
import LibrarySwitcher from '@/features/works/LibrarySwitcher.vue'
import PlayerBar from '@/features/works/PlayerBar.vue'
import RailSelect from '@/features/works/RailSelect.vue'
import RatingBar from '@/features/works/RatingBar.vue'
import TagIndex from '@/features/works/TagIndex.vue'
import { usePlayWork } from '@/features/works/use-play-work'
import { PAGE_SIZES, useWallQuery } from '@/features/works/use-wall-query'
import { useRecordWallReturn, useWallRestore } from '@/features/works/use-wall-restore'
import WorkInspector from '@/features/works/WorkInspector.vue'
import WorkPager from '@/features/works/WorkPager.vue'
import WorkWall from '@/features/works/WorkWall.vue'
import { useApiClient } from '@/lib/api/client'
import { useLibraryList } from '@/queries/libraries'
import { useWorkFacets, useWorksList, worksListQueryOptions } from '@/queries/works'
import { useLibraryUiStore } from '@/stores/library-ui'
import { usePlayerStore } from '@/stores/player'

// The work wall page: one top row (library, filter rail, SORT, FIND and the R18
// veil), the bench of one numbered page of works with its footer (count, pager,
// SIZE) and the inspector, and the player baseline — three rows inside the
// AppShell's main. A phone splits the top row into a bar (menu, library, R18)
// and the filter rail.
//
// State ownership: what the wall shows — library, filters, FIND, order, page,
// page size and the selected work — is the address's, through useWallQuery and
// its explicit actions (the library included: the store's active library is
// the workbench's, and useWallQuery writes the session's remembered Wall View).
// Where the wall was when it was left belongs to that history entry, recorded
// on every leave (use-wall-restore). The page-size
// preference only seeds a fresh visit. Typing in FIND, the tag index's staged
// picks and open menus stay in their components.

const router = useRouter()
const api = useApiClient()
const queryClient = useQueryClient()
const libraryUi = useLibraryUiStore()
const player = usePlayerStore()
const isMobile = useMediaQuery(WALL_MOBILE_QUERY)
const wall = ref<InstanceType<typeof WorkWall> | null>(null)
const wq = useWallQuery()

// ── Library: the address names it and nothing else does. The workbench's
// active library never reaches it; the address gets a library only through the
// fill (no library named: the remembered wall's, else the start library: the
// Default Library, else the newest) and the correction (a library that no
// longer exists falls back to the start library). ──
const { query: libraryQuery, librariesData, startLibraryId } = useLibraryList()
const libraryIds = computed(() => librariesData.value.map(library => library.id))
const librariesLoaded = computed(() => libraryQuery.isSuccess.value)
const noLibraries = computed(() => librariesLoaded.value && librariesData.value.length === 0)
watch([wq.library, startLibraryId, libraryIds], ([library, start]) => {
  if (!librariesLoaded.value || start === null)
    return
  const ids = libraryIds.value
  if (library === null) {
    const remembered = libraryUi.wallQuery.library
    void wq.fillLibrary(
      typeof remembered === 'string' && (remembered === 'all' || ids.includes(remembered))
        ? remembered
        : start,
    )
  }
  else if (library !== 'all' && !ids.includes(library)) {
    void wq.setLibrary(start)
  }
}, { immediate: true })
// `all` is the aggregate wall: every library's id, so the listing, the facets
// and FIND share one copy-scoped filter and a work held twice is listed once.
// A single id is trusted before the library list loads, so the first listing
// does not wait for it.
const libraryScope = computed(() => {
  const library = wq.library.value
  if (library === 'all')
    return libraryIds.value.join(',') || null
  if (library === null || (librariesLoaded.value && !libraryIds.value.includes(library)))
    return null
  return library
})

// ── Filter rail: each select reads the address and writes through an action.
// '' = no constraint. ──
const voiceActor = computed({
  get: () => wq.voice.value,
  set: value => void wq.setFilters({ voice: value }),
})
const circle = computed({
  get: () => wq.circle.value,
  set: value => void wq.setFilters({ circle: value }),
})
const age = computed({
  get: () => wq.age.value,
  set: value => void wq.setFilters({ age: value }),
})
const language = computed({
  get: () => wq.language.value,
  set: value => void wq.setFilters({ language: value }),
})
const rating = computed({
  get: () => wq.rating.value,
  set: value => void wq.setFilters({ rating: value }),
})
const listening = computed({
  get: () => wq.listening.value,
  set: value => void wq.setFilters({ listening: value }),
})
const sort = computed<WorksSort>({
  get: () => wq.sort.value,
  set: value => void wq.setSort(value),
})
const size = computed({
  get: () => String(wq.size.value),
  set: value => void wq.setSize(Number(value)),
})
const tags = computed<TagSelection>({
  get: () => ({
    groups: wq.tag.value
      ? wq.tag.value.split(',').map(group => group.split('|'))
      : [],
    mode: wq.mode.value,
  }),
  set: value => void wq.setFilters({
    tag: value.groups.map(group => group.join('|')).join(','),
    mode: value.groups.length > 0 && value.mode === 'any'
      ? 'any'
      : '',
  }),
})

// CLEAR drops every rail filter in one address commit; the library, order,
// page size and FIND are not filters of the rail.
const filtered = computed(() => [
  wq.voice,
  wq.circle,
  wq.age,
  wq.language,
  wq.rating,
  wq.listening,
  wq.tag,
].some(filter => filter.value !== ''))
function clearFilters(): void {
  void wq.setFilters({
    voice: '',
    circle: '',
    age: '',
    language: '',
    rating: '',
    listening: '',
    tag: '',
    mode: '',
  })
}

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
  // Not a language: works whose chain holds two or more language editions.
  { value: 'multi', label: '多版在库' },
]
// Personal rating is an inclusive range picked on the RatingBar, written
// `from-to` ('8-8' is exactly 8); 'none' lists the unrated. Any other address
// value, an old floor like '8' included, constrains nothing.
const RATING_OPTIONS: RailOption[] = [
  { value: '', label: 'ALL' },
  { value: 'none', label: '未评分' },
]
function ratingRange(value: string): { from: number, to: number } | null {
  const match = /^(10|[1-9])-(10|[1-9])$/.exec(value)
  if (!match)
    return null
  const from = Number(match[1])
  const to = Number(match[2])
  return from <= to
    ? { from, to }
    : null
}
function ratingParam(value: string): Pick<ListWorksParams, 'rating_from' | 'rating_to' | 'unrated'> {
  if (value === 'none')
    return { unrated: true }
  const range = ratingRange(value)
  return range
    ? { rating_from: range.from, rating_to: range.to }
    : {}
}
const ratingPicked = computed(() => ratingRange(wq.rating.value))
const ratingLabel = computed(() => {
  const range = ratingPicked.value
  if (!range)
    return undefined
  return range.from === range.to
    ? String(range.from)
    : `${range.from}–${range.to}`
})
// Listening state of the work: one pick of listening / finished / none.
const LISTENING_OPTIONS: RailOption[] = [
  { value: '', label: 'ALL' },
  { value: 'listening', label: '在听' },
  { value: 'finished', label: '听完' },
  { value: 'none', label: '未听' },
]
const AGE_PARAM: Record<string, ListWorksParams['age_category']> = { safe: 1, r15: 2, r18: 3 }

// Every order runs newest / highest first; unknown dates and unrated works
// trail in either direction.
const SORT_OPTIONS: RailOption[] = [
  { value: 'rj', label: '作品编号' },
  { value: 'release_date', label: '发售时间' },
  { value: 'collected_at', label: '入库时间' },
  { value: 'rating', label: '个人评分' },
  { value: 'listened_at', label: '最近收听' },
]
const SIZE_OPTIONS: RailOption[] = PAGE_SIZES.map(value => ({
  value: String(value),
  label: String(value),
}))

// The set of works on the wall; the order and the page only arrange it.
const scope = computed<WorksScope>(() => ({
  ...(libraryScope.value
    ? { library: libraryScope.value }
    : {}),
  ...(wq.voice.value
    ? { voice_actor: wq.voice.value, voice_actor_match: 'any' as const }
    : {}),
  ...(wq.circle.value
    ? { circle: wq.circle.value }
    : {}),
  ...(wq.language.value === 'multi'
    ? { multi_edition: true }
    : wq.language.value
      ? { language: wq.language.value }
      : {}),
  ...(wq.age.value
    ? { age_category: AGE_PARAM[wq.age.value] }
    : {}),
  ...ratingParam(wq.rating.value),
  ...(LISTENING_OPTIONS.some(option => option.value && option.value === wq.listening.value)
    ? { listening: wq.listening.value }
    : {}),
  ...(wq.tag.value
    ? { tag: wq.tag.value, tag_match: wq.mode.value }
    : {}),
  ...(wq.q.value
    ? { q: wq.q.value }
    : {}),
}))
const filters = computed<ListWorksParams>(() => ({
  ...scope.value,
  sort: wq.sort.value,
  direction: 'desc',
  page: wq.page.value,
  limit: wq.size.value,
}))
// Nothing is listed until the address names a usable library, and an address
// past the length limit is not listed: answering either with a wider listing
// would misstate what is shown.
const listable = computed(() => !wq.tooLong.value && libraryScope.value !== null)

// Each rail offers what the wall's other filters can still combine with, and
// leaves its own pick out so the alternatives to it stay on offer.
function scopeWithout(...keys: (keyof WorksScope)[]) {
  return computed(() => {
    const rest = { ...scope.value }
    for (const key of keys) delete rest[key]
    return rest
  })
}
const libraryOnly = computed<WorksScope>(() => (libraryScope.value
  ? { library: libraryScope.value }
  : {}))
/** Whether a pick has no entity left in its narrowed list, so its name must be looked up. */
function unnamed(picked: string, facets: WorkFacet[]) {
  if (picked === '')
    return false
  const ids = picked.split(',')
  return !facets.some(facet => ids.includes(facet.id))
}
const { facets: voiceActorFacets } = useWorkFacets('voice_actor', scopeWithout('voice_actor', 'voice_actor_match'), listable)
const { facets: circleFacets } = useWorkFacets('circle', scopeWithout('circle'), listable)
const { facets: voiceActorNames } = useWorkFacets('voice_actor', libraryOnly, () => unnamed(wq.voice.value, voiceActorFacets.value))
const { facets: circleNames } = useWorkFacets('circle', libraryOnly, () => unnamed(wq.circle.value, circleFacets.value))
const tagScope = scopeWithout('tag', 'tag_match')

// An entity without a source id is kept per work (ADR 0009), so one name
// can arrive as several ids. The rail offers the name once and asks for all of
// its ids: the listing matches any id of a comma-separated list. A pick the
// other filters have narrowed away stays on offer at 0 — named from the
// library's whole list, or as unknown when even that lacks it — so the tab
// still says what the wall is filtered by and ALL still clears it.
function facetOptions(facets: WorkFacet[], picked: string, names: WorkFacet[]): RailOption[] {
  const byName = new Map<string, { ids: string[], count: number }>()
  for (const facet of facets) {
    const entry = byName.get(facet.name) ?? { ids: [], count: 0 }
    entry.ids.push(facet.id)
    entry.count += facet.work_count
    byName.set(facet.name, entry)
  }
  const options = [...byName].map(([name, entry]) => ({
    value: entry.ids.join(','),
    label: name,
    count: entry.count,
  }))
  if (picked !== '' && !options.some(option => option.value === picked)) {
    const ids = picked.split(',')
    const name = [
      ...facets,
      ...names,
    ].find(facet => ids.includes(facet.id))?.name
    options.push({ value: picked, label: name ?? `未知 ×${ids.length}`, count: 0 })
  }
  return [
    { value: '', label: 'ALL' },
    ...options,
  ]
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

watch([
  settled,
  () => query.data.value,
  wq.rj,
], () => {
  const data = query.data.value
  if (!settled.value || !data)
    return
  const last = Math.max(1, Math.ceil(data.total / wq.size.value))
  if (wq.page.value > last) {
    void wq.setPage(last)
    return
  }
  if (data.works.length === 0) {
    if (wq.rj.value !== null)
      void wq.select(null)
    return
  }
  // The selection stays on the page it is shown on. A work that is not here —
  // deleted, moved off the page by an order or a concurrent change, or named
  // by a link to another page — falls back to this page, without a lookup.
  if (wq.rj.value !== null && data.works.some(work => work.rj === wq.rj.value))
    return
  const target = landOn === 'last'
    ? data.works[data.works.length - 1]
    : data.works[0]
  landOn = 'first'
  void wq.select(target.rj)
}, { immediate: true })

function turnPage(to: number, land: 'first' | 'last' = 'first', claimFocus = false): void {
  if (to < 1 || to > pageCount.value || to === wq.page.value)
    return
  landOn = land
  if (claimFocus)
    wall.value?.claimFocus()
  void wq.setPage(to)
}

// ── RND: the backend names a work and its position in this very listing (same
// filters, order and direction), so the wall lands on that page and selects it.
// A request is dropped when what it was asked about changes. ──
const toast = useToast()
const picking = ref(false)
let pickAbort: AbortController | null = null
function cancelPick(): void {
  pickAbort?.abort()
  pickAbort = null
  picking.value = false
}
watch([scope, wq.sort], cancelPick)
onBeforeUnmount(cancelPick)
const randomDisabled = computed(() => !settled.value || !listable.value || total.value === 0 || picking.value)

async function pickRandom(): Promise<void> {
  if (randomDisabled.value)
    return
  const abort = pickAbort = new AbortController()
  picking.value = true
  try {
    const { rj, index } = await api.randomWork({
      ...scope.value,
      sort: wq.sort.value,
      direction: 'desc',
      ...(selectedRj.value
        ? { exclude: selectedRj.value }
        : {}),
    }, abort.signal)
    if (abort.signal.aborted)
      return
    const to = Math.floor(index / wq.size.value) + 1
    // The page on show lacks the pick (stale): refresh it first, or the
    // correction above would settle the selection back on its first work.
    if (to === wq.page.value && !works.value.some(work => work.rj === rj)) {
      await query.refetch()
      if (abort.signal.aborted)
        return
    }
    await wq.jumpTo(to, rj)
  }
  catch (e) {
    if (!abort.signal.aborted)
      toast.error(e)
  }
  finally {
    if (pickAbort === abort) {
      pickAbort = null
      picking.value = false
    }
  }
}

// ── FIND: the box is a draft; its debounced text is the address's `q`. An
// address change replaces the draft and cancels a pending write. ──
const findText = ref(wq.q.value)
let findTimer: number | null = null
let findSubmission = 0
function cancelFind(): void {
  findSubmission++
  if (findTimer !== null)
    window.clearTimeout(findTimer)
  findTimer = null
}
watch(findText, (value) => {
  cancelFind()
  if (value.trim() === wq.q.value)
    return
  findTimer = window.setTimeout(() => {
    findTimer = null
    void wq.setFilters({
      q: value.trim(),
    })
  }, 200)
})
watch(wq.q, (q) => {
  if (q === findText.value.trim())
    return
  cancelFind()
  findText.value = q
})
onBeforeUnmount(cancelFind)

async function submitFind(event: KeyboardEvent): Promise<void> {
  if (event.isComposing || event.keyCode === 229)
    return
  const text = findText.value.trim()
  const code = canonicalRj(text)
  if (!code)
    return
  event.preventDefault()
  cancelFind()
  const submission = findSubmission
  // The address first, then the listing of exactly that address.
  await wq.setFilters({ q: text })
  let data
  try {
    data = await queryClient.fetchQuery(worksListQueryOptions(api, filters.value))
  }
  catch {
    // ponytail: the failed listing surfaces through the wall's own query.
    return
  }
  if (submission !== findSubmission)
    return
  const collected = data.works.filter(candidate => candidate.copy_count > 0)
  const exact = collected.find(candidate => candidate.rj === code)
  if (exact)
    await openWork(exact.rj)
  else if (data.total === data.works.length && collected.length === 1)
    await openWork(collected[0].rj)
}

// ── Opening a work: every entry (click, tap, Enter, FIND) commits the work to
// the address and then leaves. ──
async function openWork(rj: string): Promise<void> {
  await wq.select(rj)
  await router.push({
    name: 'work-detail',
    params: { rj },
  })
}

// Every way off the wall records where it was, so BACK returns to this place.
const recordReturn = useRecordWallReturn()
onBeforeRouteLeave(() => {
  recordReturn({ scrollTop: wall.value?.scroller?.scrollTop ?? 0 })
})

useWallRestore({
  ready: () => settled.value,
  wall: wall as Ref<WallHandle | null>,
  selectedRj: () => wq.rj.value,
})

// ── Keyboard: the wall's shortcuts register with the keymap. ──
const { playWork } = usePlayWork()
function playSelected(): void {
  const work = works.value.find(candidate => candidate.rj === selectedRj.value)
  if (work)
    void playWork({ rj: work.rj, title: work.title ?? work.rj })
}

// defineExpose unwraps the computed, so the instance carries the number.
useKeymap({
  'wall.move': (event) => {
    const cols = wall.value?.columns ?? 1
    const moves: Record<string, number> = {
      ArrowRight: 1,
      ArrowLeft: -1,
      ArrowDown: cols,
      ArrowUp: -cols,
    }
    wall.value?.move(moves[event.key])
  },
  'wall.page': event => void turnPage(wq.page.value + (event.key === 'PageDown'
    ? 1
    : -1), 'first', true),
  'wall.play': playSelected,
  // Space is the player's: browsing never interrupts playback.
  'wall.open': () => (selectedRj.value === null
    ? false
    : void openWork(selectedRj.value)),
})
</script>

<template>
  <section class="works substrate" aria-label="作品库">
    <div class="top" :class="{ filtered }">
      <div class="header-left">
        <GlobalMenu />
        <LibrarySwitcher :library="wq.library.value" @select="wq.setLibrary" />
      </div>
      <div class="filters" role="group" aria-label="Filter">
        <RailSelect v-model="voiceActor" label="VOICE" :options="voiceActorOptions" marks-active searchable />
        <RailSelect v-model="circle" label="CIRCLE" :options="circleOptions" marks-active searchable />
        <RailSelect v-model="age" label="AGE" :options="AGE_OPTIONS" marks-active />
        <RailSelect v-model="language" label="LANG" :options="LANGUAGE_OPTIONS" marks-active />
        <RailSelect v-model="rating" label="RATING" :options="RATING_OPTIONS" :value-label="ratingLabel" marks-active>
          <template #default="{ open, choose }">
            <RatingBar :range="ratingPicked" :open="open" @pick="(from, to) => choose(`${from}-${to}`)" />
          </template>
        </RailSelect>
        <RailSelect v-model="listening" label="PROG" :options="LISTENING_OPTIONS" marks-active />
        <TagIndex v-model="tags" :scope="tagScope" />
        <span v-if="wq.overflow.value" class="lbl lbl-xs notice" role="status">筛选条件过多，未应用</span>
      </div>
      <!-- Outside .filters: a phone scrolls that row, and CLEAR must stay in view. -->
      <button v-if="filtered" class="lbl rail-clear" type="button" @click="clearFilters">CLEAR</button>
      <div class="rail-right">
        <RailSelect v-model="sort" class="sort" label="SORT" :options="SORT_OPTIONS" />
        <label class="find">
          <span class="lbl">FIND</span>
          <input v-model="findText" type="text" placeholder="RJ / TITLE" @keydown.enter="submitFind">
        </label>
        <button
          class="btn btn-s veil" type="button" :aria-pressed="libraryUi.veil"
          title="观看者选项：R18 作品的封面与插画先以排线遮盖"
          @click="libraryUi.toggleVeil()"
        >R18<span class="veil-state">{{ libraryUi.veil ? ' ON' : ' OFF' }}</span></button>
      </div>
    </div>

    <main class="bench">
      <div class="wall-col">
        <div class="wall-note">
          <p v-if="wq.tooLong.value" class="lbl lbl-xs notice" role="alert">地址中的筛选条件过长，未列出作品；请在筛选栏清除部分条件。</p>
          <p v-else-if="noLibraries || (settled && total === 0)" class="lbl lbl-xs notice">没有作品</p>
        </div>
        <WorkWall
          ref="wall" :works="works" :selected-rj="selectedRj" :playing-rj="player.rj"
          @select="wq.select($event)" @open="openWork"
          @edge="(direction) => turnPage(wq.page.value + direction, direction < 0 ? 'last' : 'first', true)"
        />
        <footer class="wall-foot">
          <span class="lbl count">WORKS<b>{{ total }}</b></span>
          <WorkPager :page="wq.page.value" :page-count="pageCount" @go="turnPage" />
          <RailSelect v-model="size" class="size" label="SIZE" :options="SIZE_OPTIONS" up />
        </footer>
      </div>
      <!-- A phone has no inspector: a tap routes to the detail page instead. -->
      <WorkInspector v-if="!isMobile" :rj="selectedRj" :random-disabled="randomDisabled" @random="pickRandom" />
    </main>

    <PlayerBar />
  </section>
</template>

<style scoped>
/* App rows: top row (library, filters, SORT, FIND, R18) min 48, work area 1fr,
   bottom bar 28. The top row's lower edge and the bottom bar's upper edge are
   the two outer edges and carry the only double bar (1px fine + 2px strong);
   everything inside uses single 1px lines. A phone splits the top row into a
   48px bar (menu, library, R18) that keeps the double bar, and the rail. */
.works {
  display: grid; grid-template-rows: minmax(48px, auto) minmax(0, 1fr) auto; grid-template-columns: minmax(0, 1fr);
  width: 100%; height: 100%;
  font-variant-numeric: tabular-nums;
}

.top {
  display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-1) var(--sp-4);
  padding: var(--sp-1) var(--sp-6); position: relative;
}
.top::after {
  content: ''; position: absolute; left: var(--sp-6); right: var(--sp-6); bottom: 0; height: 4px;
  border-top: 1px solid var(--wire-fine); border-bottom: 2px solid var(--wire-strong);
}
.header-left, .filters, .rail-right { display: flex; align-items: center; gap: var(--sp-1); min-width: 0; }
.header-left { gap: var(--sp-2); flex-shrink: 0; }
/* Too narrow for one line, SORT/FIND/R18 drop to their own line on the right,
   leaving the filters the full width; only then do the tabs themselves wrap (a
   set filter widens its tab by its value). */
.rail-right { gap: var(--sp-4); flex-shrink: 0; margin-left: auto; }
.filters { flex-wrap: wrap; row-gap: var(--sp-1); }

/* Beside the tabs, as if the last of them: the row's gap is wider than theirs. */
.rail-clear {
  height: 24px; padding: 0 var(--sp-3); margin-left: calc(var(--sp-1) - var(--sp-4)); white-space: nowrap;
  color: var(--fg-muted);
}
.rail-clear:hover { color: var(--fg-base); }
.rail-clear:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 2px; }
@media (pointer: coarse) {
  .rail-clear { height: auto; min-height: 44px; }
}

@media (min-width: 769px) {
  .veil { height: 24px; }
}
.find { display: flex; align-items: center; gap: var(--sp-2); flex-shrink: 0; }
.find input {
  height: 24px; width: 184px; padding: 0 var(--sp-2);
  border: 1px solid var(--wire-control); background: transparent; color: var(--fg-base);
  font-family: var(--font-mono); font-size: var(--text-xs); letter-spacing: var(--track-label);
}
.find input::placeholder { color: var(--fg-meta); }
.find input:focus { border-color: var(--wire-strong); outline: none; }

.bench {
  display: grid; grid-template-columns: minmax(0, 1fr) clamp(400px, 30vw, 560px);
  min-height: 0;
}
/* The wall scrolls; the footer stays under it. */
.wall-col { display: grid; grid-template-rows: auto minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
.notice { padding: var(--sp-3) var(--sp-6) 0; letter-spacing: var(--track-data); text-transform: none; }
.top .notice { padding: 0 var(--sp-2); }

/* Count left, pager centred, SIZE right; the pager row may be absent (one page)
   and the row keeps its height and both ends. */
.wall-foot {
  position: relative; display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center;
  min-height: 40px; padding: 0 var(--sp-6);
}
.wall-foot::before {
  content: ''; position: absolute; left: var(--sp-6); right: var(--sp-6); top: 0;
  border-top: 1px solid var(--wire-fine);
}
.wall-foot .count { grid-column: 1; }
.wall-foot .count b { color: var(--fg-base); font-weight: 600; letter-spacing: var(--track-label); }
.size { grid-column: 3; justify-self: end; }

@media (max-width: 768px) {
  /* A phone's rail cannot hold every tab on one line: the filters scroll
     sideways in one row, SORT stays pinned beside them, and FIND takes its own
     line. The scrolling row clips an absolute dropdown, which is fine: a
     phone's panels are modal sheets in the top layer. The top row is a grid:
     the bar (menu, library, R18) on a 48px first row that keeps the double bar,
     then the filters + SORT, then CLEAR + FIND. */
  .works { grid-template-rows: auto minmax(0, 1fr) auto; }
  .top {
    display: grid; grid-template-columns: auto minmax(0, 1fr) auto; grid-template-rows: 48px;
    align-items: center; gap: var(--sp-2) var(--sp-1); padding: 0 var(--sp-4) var(--sp-2);
  }
  .top::after { left: var(--sp-4); right: var(--sp-4); height: 0; border-top: 0; border-bottom: 1px solid var(--wire-fine); }
  .header-left { grid-area: 1 / 1 / 2 / -1; align-self: stretch; position: relative; }
  .header-left::after {
    content: ''; position: absolute; left: 0; right: 0; bottom: 0; height: 4px;
    border-top: 1px solid var(--wire-fine); border-bottom: 2px solid var(--wire-strong);
  }
  .rail-right { display: contents; }
  .veil { grid-area: 1 / 3; justify-self: end; z-index: 1; }
  .veil-state { display: none; }
  .filters { grid-area: 2 / 1 / 3 / 3; flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; }
  .filters::-webkit-scrollbar { display: none; }
  .filters > * { flex-shrink: 0; }
  .sort { grid-area: 2 / 3; }
  /* The first column exists for CLEAR, pinned at the start of FIND's line; with
     no CLEAR it is empty and FIND spans the whole line. */
  .rail-clear { grid-area: 3 / 1; margin-left: 0; padding: 0 var(--sp-2); }
  .find { grid-area: 3 / 1 / auto / -1; min-width: 0; padding-left: var(--sp-2); }
  .top.filtered .find { grid-area: 3 / 2 / auto / -1; padding-left: 0; }
  .find input { flex: 1; min-width: 0; width: auto; }
  .bench { grid-template-columns: 1fr; }
  /* A phone keeps the default page size and no count: the pager alone. */
  .wall-foot { display: flex; justify-content: center; min-height: 0; padding: 0 var(--sp-4); }
  .wall-foot::before, .wall-foot .count, .size { display: none; }
}
</style>
