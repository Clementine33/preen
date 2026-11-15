<script setup lang="ts">
import type { WorkEditionNode, WorkEditionSlot, WorkFacetKind, WorkUserTag } from '@/lib/api/types'
import { Heart, LayersPlus } from '@lucide/vue'
import { useQueryClient } from '@tanstack/vue-query'
import { onClickOutside } from '@vueuse/core'
import { computed, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useToast } from '@/composables/use-toast'
import { dottedDate, fmtT, resumePoint } from '@/features/work-detail/derive'
import { useApiClient } from '@/lib/api/client'
import { newCollectionName, useCollectionManage, useCollectionMemberWrites, useCollections } from '@/queries/collections'
import { useLibraryList } from '@/queries/libraries'
import { useCreateUserTag, useReplaceWorkUserTags, useUpdateWorkUserData, useUserTags, useWorkDetail, workFacetsQueryOptions } from '@/queries/works'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'

// The colophon (奥付, v3): the work as a printed edition — ruled rows of what
// the catalog knows, then the personal rows, written straight through (rating,
// ♥ FAV, my tags, the `+ 收藏夹` add-to menu). The
// plate's reverse renders the same component compacted (`compact`).

const props = defineProps<{
  /** The plate's back face: tighter padding, smaller title, shorter note. */
  compact?: boolean
}>()

const store = useWorkDetailStore()
const { work } = useWorkDetail(() => store.rj)
const { librariesData } = useLibraryList()
const { tags: allTags } = useUserTags()
const player = usePlayerStore()
const userData = useUpdateWorkUserData(() => store.rj)
const tagReplace = useReplaceWorkUserTags(() => store.rj)
const tagCreate = useCreateUserTag()

const rating = computed(() => work.value?.rating ?? null)
const favorite = computed(() => work.value?.favorite ?? false)
const mine = computed<WorkUserTag[]>(() => work.value?.user_tags ?? [])

const toast = useToast()
const api = useApiClient()
const queryClient = useQueryClient()
const router = useRouter()

/**
 * Open the wall on one circle or voice actor. The wall filters by entity id,
 * and an entity without a source id is one id per work (ADR 0009), so the
 * name is resolved to every id carrying it, as the filter rail does. `own` is
 * this work's id for the name, used when the lookup finds none; a circle has no id here.
 */
async function showOnWall(kind: WorkFacetKind, name: string, own?: string): Promise<void> {
  try {
    const ids = (await queryClient.fetchQuery(workFacetsQueryOptions(api, kind, {}))).filter(facet => facet.name === name).map(facet => facet.id)
    const picked = (ids.length > 0
      ? ids
      : own
        ? [own]
        : []).join(',')
    if (picked !== '') {
      await router.push({
        name: 'works',
        query: { [kind === 'circle'
          ? 'circle'
          : 'voice']: picked },
      })
    }
  }
  catch (e) { toast.error(e) }
}

const age = computed(() =>
  work.value?.age_category
    ? ({ 1: '全年龄', 2: 'R15', 3: 'R18' }[work.value.age_category])
    : null,
)

function setRating(n: number): void {
  // Clicking the set value clears it (rating null is the unrated state).
  userData.mutate({
    rating: rating.value === n
      ? null
      : n,
  })
}

function toggleFavorite(): void {
  userData.mutate({ favorite: !favorite.value })
}

// ── Add to collection: tick follows `collection_ids`; a click joins or leaves, no undo here. ──
const { collections } = useCollections()
const memberWrites = useCollectionMemberWrites()
const manage = useCollectionManage()
const picking = ref(false)
const rateRow = ref<HTMLElement | null>(null)
const userCollections = computed(() => collections.value.filter(c => c.id !== 'favorites'))
const held = computed(() => work.value?.collection_ids ?? [])

async function toggleCollection(id: string, name: string): Promise<void> {
  const rj = store.rj
  if (rj === null)
    return
  try {
    if (held.value.includes(id)) {
      await memberWrites.remove(id, rj)
      toast.show({ text: `已从 ${name} 移除` })
    }
    else {
      await memberWrites.append(id, [rj])
      toast.show({ text: `已添加到 ${name}` })
    }
  }
  catch (e) { toast.error(e) }
}

async function addToNewCollection(): Promise<void> {
  const rj = store.rj
  if (rj === null)
    return
  try {
    const made = await manage.create(newCollectionName(collections.value))
    await memberWrites.append(made.id, [rj])
    toast.show({ text: `已新建 ${made.name} 并添加` })
    picking.value = false
  }
  catch (e) { toast.error(e) }
}

// ── Listening Position: 继续 plays the saved track at its second; with nothing
// to resume, 重听 (a Finished work) or 从头播放 (only orphans left) starts track 1.
// 标记为未听 forgets every Rendition's. ──
const resume = computed(() => resumePoint(work.value?.listening ?? [], store.tracks))
const restartLabel = computed(() => (work.value?.listening.some(l => l.finished_at !== null)
  ? '重听'
  : '从头播放'))

function resumeListening(): void {
  store.startTrack(resume.value?.track ?? 0, undefined, resume.value?.position)
}

async function clearListening(): Promise<void> {
  if (store.rj === null)
    return
  try {
    await player.clearListening(store.rj)
  }
  catch (e) { toast.error(e) }
}

function saveNote(event: Event): void {
  const value = (event.target as HTMLTextAreaElement).value
  if (work.value && value !== work.value.note)
    userData.mutate({ note: value })
}

// ── My tags: pick from the vocabulary, or create a tag by name (idempotent). ──
const adding = ref(false)
const tagRow = ref<HTMLElement | null>(null)
onClickOutside(rateRow, () => { picking.value = false })
onClickOutside(tagRow, () => { adding.value = false })
const newTag = ref('')
const addable = computed(() => allTags.value.filter(tag => !mine.value.some(entry => entry.id === tag.id)))

function addTag(id: string): void {
  adding.value = false
  tagReplace.mutate([
    ...mine.value.map(tag => tag.id),
    id,
  ])
}

function removeTag(id: string): void {
  tagReplace.mutate(mine.value.filter(tag => tag.id !== id).map(tag => tag.id))
}

async function createTag(): Promise<void> {
  const name = newTag.value.trim()
  if (name === '')
    return
  const tag = await tagCreate.mutateAsync(name)
  newTag.value = ''
  addTag(tag.id)
}

/** A catalog date or timestamp, as `YYYY.MM.DD`. */
function dotted(value: string | null): string {
  return dottedDate(value === null
    ? null
    : value.slice(0, 10))
}

/** A copy's online square: the manifest knows, the catalog listing does not. */
function copyOnline(copyId: string): boolean | null {
  return store.content?.copies.find(copy => copy.copy_id === copyId)?.online ?? null
}

const SLOT_NAMES: Record<string, string> = { JPN: '日语', CHI_HANS: '简中', CHI_HANT: '繁中' }
const ROLE_NAMES: Record<WorkEditionNode['role'], string> = { original: '原作', edition_page: '页面', translation: '翻译' }

/**
 * The 版本 row's lines. A slot is current on its own page or on the page of
 * the translation being read; the compact back keeps only each slot's root
 * plus this work's own translation node.
 */
const editionLines = computed(() => {
  const self = work.value?.rj
  const parent = work.value?.relation?.parent_rj
  return (work.value?.edition_tree ?? []).map((slot: WorkEditionSlot) => ({
    slot,
    current: slot.root.rj === self || slot.root.rj === parent,
    nodes: [
      slot.root,
      ...(props.compact
        ? slot.translations.filter(node => node.rj === self)
        : slot.translations),
    ],
  }))
})

/** A copy's library link: its folder, or the library overview when offline. */
function copyTo(copy: { copy_id: string, library_id: string, dir_id: string }) {
  return copyOnline(copy.copy_id) === false || copy.dir_id === ''
    ? {
        name: 'workbench-overview',
        params: { libraryId: copy.library_id },
      }
    : {
        name: 'overview-files',
        params: { libraryId: copy.library_id, dirId: copy.dir_id },
      }
}
</script>

<template>
  <div class="colo" :class="{ compact: props.compact }">
    <h2 class="colo-title" :lang="work?.relation?.language === 'CHI_HANS' ? 'zh-Hans' : work?.relation?.language === 'CHI_HANT' ? 'zh-Hant' : 'ja'">{{ work?.title ?? '—' }}</h2>

    <dl class="colo-box">
      <div class="colo-row"><dt>作品</dt><dd><span class="mono">{{ work?.rj ?? '—' }}</span></dd></div>
      <div class="colo-row"><dt>社团</dt><dd>
        <button v-if="work?.circle" class="jump" type="button" lang="ja" @click="showOnWall('circle', work.circle)">{{ work.circle }}</button>
        <span v-else>—</span>
      </dd></div>
      <div class="colo-row"><dt>声优</dt><dd>
        <template v-for="(actor, index) in work?.voice_actors ?? []" :key="actor.id">
          <span v-if="index > 0" class="sep">/</span>
          <button class="jump" type="button" lang="ja" @click="showOnWall('voice_actor', actor.name, actor.id)">{{ actor.name }}</button>
        </template>
        <span v-if="!work?.voice_actors.length">—</span>
      </dd></div>
      <div class="colo-row"><dt>发售</dt>
        <dd><span class="mono">{{ dottedDate(work?.release_date ?? null) }}</span>
          <template v-if="work?.dl_count !== null && work?.dl_count !== undefined"><span class="sep">·</span><span class="lbl lbl-xs">DL<b>{{ work.dl_count.toLocaleString() }}</b></span></template>
        </dd>
      </div>
      <div v-if="age" class="colo-row"><dt>分级</dt>
        <dd><span class="mono">{{ age }}</span></dd>
      </div>
      <div v-if="work?.relation_warnings?.length" class="colo-row"><dt>关系</dt>
        <dd><span class="rel-warn" role="note"><span v-for="warning in work.relation_warnings" :key="`${warning.code}-${warning.rj}`">{{ warning.rj }} · {{ warning.message }}</span></span></dd>
      </div>
      <div v-if="work?.series_name" class="colo-row"><dt>系列</dt><dd><span lang="ja">{{ work.series_name }}</span></dd></div>
      <div v-if="editionLines.length" class="colo-row"><dt>版本</dt>
        <dd><span class="ed">
          <span
            v-for="line in editionLines" :key="line.slot.language" class="ed-line"
            :aria-current="line.current ? 'true' : undefined"
          >
            <span class="lbl lbl-xs">{{ SLOT_NAMES[line.slot.language] ?? line.slot.language }}</span>
            <template v-for="(node, index) in line.nodes" :key="node.rj">
              <span v-if="index > 0" class="sep">{{ index === 1 ? '→' : '·' }}</span>
              <span class="tag" :aria-current="node.rj === work?.rj ? 'true' : undefined">
                {{ ROLE_NAMES[node.role] }}
                <RouterLink v-if="node.is_work && node.rj !== work?.rj" class="mono" :to="{ name: 'work-detail', params: { rj: node.rj } }">{{ node.rj }}</RouterLink>
                <span v-else class="mono">{{ node.rj }}</span>
                <span v-if="node.held" class="lbl">HELD</span>
              </span>
            </template>
            <!-- The compact back drops the translations, so a slot held only through one says so itself. -->
            <span v-if="props.compact && line.slot.held && !line.nodes.some((node) => node.held)" class="lbl">HELD</span>
          </span>
        </span></dd>
      </div>
      <div v-if="work?.source_tags.length" class="colo-row"><dt>标签</dt>
        <dd><span class="tags"><span v-for="tag in work.source_tags" :key="tag.id" class="tag plain">{{ tag.name }}</span></span></dd>
      </div>
      <div v-if="work?.copies.length" class="colo-row"><dt>副本</dt>
        <dd><span class="copy">
          <template v-for="copy in work.copies" :key="copy.copy_id">
            <span><RouterLink :to="copyTo(copy)">{{ librariesData.find(library => library.id === copy.library_id)?.name ?? '—' }}</RouterLink><template v-if="copyOnline(copy.copy_id) === false"><span class="sep">·</span><span class="lbl">OFFLINE</span></template></span>
          </template>
        </span></dd>
      </div>
      <div class="colo-row"><dt>收录</dt><dd><span class="mono">{{ dotted(work?.collected_at ?? null) }}</span></dd></div>
      <div v-if="work?.source_url" class="colo-row"><dt>来源</dt>
        <dd><a :href="work.source_url" target="_blank" rel="noreferrer">{{ work.source_url.replace('https://', '') }}</a></dd>
      </div>
    </dl>

    <span class="lbl">PERSONAL</span>
    <dl class="colo-box">
      <div class="colo-row"><dt>评分</dt>
        <dd ref="rateRow" class="acts">
          <span class="rating">
            <i
              v-for="n in 10" :key="n" :class="{ on: rating !== null && n <= rating }"
              role="button" tabindex="0"
              :aria-label="`评分 ${n}`" @click="setRating(n)" @keydown.enter="setRating(n)"
            />
          </span>
          <span class="lbl lbl-xs"><b>{{ rating ?? '—' }}</b>/10</span>
          <button class="fav icon" type="button" title="喜欢" aria-label="喜欢" :aria-pressed="favorite" @click="toggleFavorite"><Heart aria-hidden="true" :fill="favorite ? 'currentColor' : 'none'" /></button>
          <button class="fav icon" type="button" title="加入收藏夹" aria-label="加入收藏夹" data-testid="work-collections" :aria-expanded="picking" @click="picking = !picking"><LayersPlus aria-hidden="true" /></button>
          <span v-if="picking" class="menu" role="menu">
            <div class="strip"><span class="strip-name lbl">ADD TO COLLECTION</span></div>
            <button
              v-for="c in userCollections" :key="c.id" class="menu-item" type="button" role="menuitemcheckbox"
              :aria-checked="held.includes(c.id)" data-testid="work-collection-item" :data-id="c.id" @click="toggleCollection(c.id, c.name)"
            ><span class="glyph">{{ held.includes(c.id) ? '■' : '□' }}</span>{{ c.name }}</button>
            <button class="menu-item new-tag" type="button" role="menuitem" data-testid="work-collection-new" @click="addToNewCollection">
              <span class="glyph">＋</span>新建收藏夹
            </button>
          </span>
        </dd>
      </div>
      <div v-if="work?.listening.length" class="colo-row"><dt>收听</dt>
        <dd class="acts">
          <button class="fav" type="button" :disabled="store.tracks.length === 0" @click="resumeListening"><template v-if="resume">▶ 继续 · {{ resume.row.track_index! + 1 }}/{{ resume.row.track_count }} · {{ fmtT(resume.position) }}</template><template v-else>▶ {{ restartLabel }}</template></button>
          <button class="fav" type="button" @click="clearListening">标记为未听</button>
        </dd>
      </div>
      <div class="colo-row"><dt>我的</dt>
        <dd ref="tagRow">
          <span class="tags">
            <button
              v-for="tag in mine" :key="tag.id" class="tag mine" type="button"
              title="移除这个标签" @click="removeTag(tag.id)"
            >{{ tag.name }}</button>
            <button
              class="tag add" :class="{ quiet: mine.length > 0 && !adding }" type="button" :aria-expanded="adding"
              @click="adding = !adding"
            >+ TAG</button>
          </span>
          <span v-if="adding" class="menu" role="menu">
            <div class="strip"><span class="strip-name lbl">ADD TAG</span></div>
            <button
              v-for="tag in addable" :key="tag.id" class="menu-item" type="button" role="menuitem"
              @click="addTag(tag.id)"
            ><span class="glyph">¶</span>{{ tag.name }}</button>
            <form class="menu-item new-tag" @submit.prevent="createTag">
              <span class="glyph">＋</span>
              <input v-model="newTag" data-own-keys type="text" placeholder="NEW TAG">
            </form>
          </span>
        </dd>
      </div>
      <div class="colo-row"><dt>笔记</dt>
        <dd><textarea
          class="note" :rows="props.compact ? 2 : 3" :value="work?.note ?? ''"
          @change="saveNote"
        /></dd></div>
    </dl>

    <div class="colo-foot">
      <span class="lbl">{{ props.compact ? 'FLIP BACK ◢' : '' }}</span>
    </div>
  </div>
</template>

<style scoped>
.colo { padding: var(--sp-5) var(--sp-5) var(--sp-6); display: flex; flex-direction: column; gap: var(--sp-4); }
.colo.compact { padding: var(--sp-4); gap: var(--sp-3); }
.colo.compact .colo-title { font-size: var(--text-sm); line-height: var(--text-sm--line-height); }
.colo-title { font-family: var(--font-mincho); font-size: var(--text-xl); line-height: var(--text-xl--line-height); font-weight: 700; letter-spacing: 0; }
/* A printed 奥付: one thin rule under the title, unruled rows, labels justified to a common width. */
.colo-title + .colo-box { border-top: 1px solid var(--wire-mid); padding-top: var(--sp-3); }
.colo-row { font-size: var(--text-sm); display: grid; grid-template-columns: 3em 1fr; gap: var(--sp-4); align-items: baseline; padding: 5px 0; }
.colo-row dt { color: var(--fg-muted); text-align-last: justify; white-space: nowrap; }
.colo-row dd { font-weight: 500; min-width: 0; position: relative; }
.mono { font-family: var(--font-mono); font-size: var(--text-xs); letter-spacing: var(--track-label); }
/* Only the source URL is a bare link in mono; the RJ links carry `.mono`, and a copy is named by its Library (user-written, may hold CJK), so it stays in the row's own face. */
.colo-row dd > a { font-family: var(--font-mono); font-size: var(--text-xs); word-break: break-all; }
/* Overrides the global `.tags` input-field box (theme-yorha.css). */
.tags { display: flex; flex-wrap: wrap; gap: 4px; min-height: 0; padding: 0; background: none; border: none; }
.tags:focus-within { box-shadow: none; }
.tag { min-height: 22px; padding: 0 6px; display: inline-flex; align-items: center; border: 1px solid transparent; font-size: var(--text-sm); }
.tag.add.quiet { opacity: 0; }
.colo-row:hover .tag.add.quiet, .tag.add.quiet:focus-visible { opacity: 1; }
.tag[aria-current='true'] { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }
.tag.mine:hover { text-decoration: line-through; }
.tag.plain { background: none; padding: 0; margin-right: var(--sp-2); } /* plain text, flush with the value column */
.tag.add { border: 1px dashed var(--wire-mid); color: var(--fg-meta); font-family: var(--font-mono); font-size: var(--text-xs); }
.ed { display: flex; flex-direction: column; gap: 4px; }
.rel-warn { display: flex; flex-direction: column; gap: 2px; color: var(--warning-ink); }
.ed-line { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; }
.ed-line[aria-current='true'] > .lbl { color: var(--fg-base); font-weight: 700; }
.tag .lbl { margin-left: 4px; }
.colo-row dd a { color: inherit; text-underline-offset: 3px; }
.jump { font: inherit; color: inherit; background: none; border: 0; padding: 0; cursor: pointer; text-underline-offset: 3px; }
.colo-row dd :is(a, .jump):is(:hover, :focus-visible) { color: var(--info-ink); text-decoration: underline; }
.tag a { margin-left: 4px; }
.copy { display: grid; grid-template-columns: auto 1fr; gap: 2px var(--sp-2); }
.copy > span:nth-child(even) { min-width: 0; overflow-wrap: anywhere; }
.acts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-1) var(--sp-2); }
.rating { display: inline-flex; gap: 4px; }
.rating i { width: 14px; height: 14px; border: 1px solid var(--wire-mid); cursor: pointer; }
.rating i.on { background: var(--invert-bg); border-color: var(--invert-bg); }
.rating i:hover { border-color: var(--wire-strong); }
.fav { font-family: var(--font-mono); font-size: var(--text-xs); letter-spacing: var(--track-caps); min-height: 18px; padding: 0 6px; border: 1px solid var(--wire-mid); color: var(--fg-base); background: none; cursor: pointer; }
/* An icon-only fav: a square of the same 22px as a tag, glyph 14px on the centre line. */
.fav.icon { width: 22px; height: 22px; padding: 0; display: inline-grid; place-items: center; }
.fav.icon svg { width: 14px; height: 14px; }
.fav[aria-pressed='true'] { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }
.note { width: 100%; min-height: 64px; resize: vertical; background: transparent; border: 1px solid var(--wire-fine); padding: var(--sp-2); font-size: var(--text-xs); line-height: 20px; color: var(--fg-base); outline: none; }
.note:focus { border-color: var(--wire-strong); }
.note:disabled { opacity: .5; }
.colo-foot { display: flex; justify-content: space-between; }
/* The + TAG specimen panel, same grammar as the leaf's pair menu. */
.menu { position: absolute; left: 0; top: calc(100% + 8px); z-index: 20; width: 264px; padding-bottom: var(--sp-2); background: var(--grain), var(--bg-panel); border: 1px solid var(--wire-strong); box-shadow: 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid); }
.menu .strip { margin: var(--sp-2) var(--sp-2) var(--sp-1); }
.menu-item { display: flex; align-items: center; gap: var(--sp-2); width: calc(100% - 16px); margin: 0 var(--sp-2); height: 28px; padding: 0 var(--sp-2) 0 var(--sp-1); border-bottom: 1px solid var(--wire-fine); font-size: var(--text-xs); text-align: left; color: var(--fg-base); background: none; cursor: pointer; }
.menu-item:hover { background: var(--invert-bg); color: var(--invert-fg); }
.menu-item .glyph { width: 14px; text-align: center; font-family: var(--font-mono); font-size: var(--text-2xs); color: inherit; }
.menu-item.new-tag { border-bottom: none; }
.menu-item input { flex: 1; min-width: 0; border: none; border-bottom: 1px solid var(--wire-mid); background: transparent; color: inherit; font-size: inherit; outline: none; }
/* A narrower column (a phone, a pinned bay) fits the ten boxes, the score and both icons on one line only with smaller boxes. */
@media (max-width: 1100px) {
  .rating { gap: 3px; }
  .rating i { width: 12px; height: 12px; }
}
</style>
