<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useToast } from '@/composables/use-toast'
import { newCollectionName, useCollectionManage, useCollectionMemberWrites, useCollections } from '@/queries/collections'
import { dottedDate } from '@/features/work-detail/derive'
import { useWorkDetail, useCreateUserTag, useReplaceWorkUserTags, useUpdateWorkUserData, useUserTags } from '@/queries/works'
import { useLibraryList } from '@/queries/libraries'
import { useWorkDetailStore } from '@/stores/work-detail'
import type { WorkEditionNode, WorkEditionSlot, WorkUserTag } from '@/lib/api/types'

// The colophon (奥付, v3): the work as a printed edition — ruled rows of what
// the catalog knows, then the personal rows, written straight through (rating,
// ♥ FAV, my tags, the `+ 合集` add-to menu). The
// plate's reverse renders the same component compacted (`compact`).

const props = defineProps<{
  /** The plate's back face: tighter padding, smaller title, shorter note. */
  compact?: boolean
}>()

const store = useWorkDetailStore()
const { work } = useWorkDetail(() => store.rj)
const { librariesData } = useLibraryList()
const { tags: allTags } = useUserTags()
const userData = useUpdateWorkUserData(() => store.rj)
const tagReplace = useReplaceWorkUserTags(() => store.rj)
const tagCreate = useCreateUserTag()

// Placeholder works refuse personal-data writes (WORK_IS_PLACEHOLDER, docs/api.md).
const locked = computed(() => work.value?.placeholder ?? false)
const rating = computed(() => work.value?.rating ?? null)
const favorite = computed(() => work.value?.favorite ?? false)
const mine = computed<WorkUserTag[]>(() => work.value?.user_tags ?? [])

const voice = computed(() =>
  work.value && work.value.voice_actors.length > 0 ? work.value.voice_actors.map((actor) => actor.name).join(' / ') : '—',
)
const age = computed(() =>
  work.value?.age_category ? ({ 1: '全年龄', 2: 'R15', 3: 'R18' }[work.value.age_category]) : null,
)

function setRating(n: number): void {
  if (locked.value) return
  // Clicking the set value clears it (rating null is the unrated state).
  userData.mutate({ rating: rating.value === n ? null : n })
}

function toggleFavorite(): void {
  if (locked.value) return
  userData.mutate({ favorite: !favorite.value })
}

// ── Add to collection: tick follows `collection_ids`; a click joins or leaves, no undo here. ──
const toast = useToast()
const { collections } = useCollections()
const memberWrites = useCollectionMemberWrites()
const manage = useCollectionManage()
const userCollections = computed(() => collections.value.filter((c) => c.id !== 'favorites'))
const held = computed(() => work.value?.collection_ids ?? [])

async function toggleCollection(id: string, name: string): Promise<void> {
  const rj = store.rj
  if (locked.value || rj === null) return
  try {
    if (held.value.includes(id)) {
      await memberWrites.remove(id, rj)
      toast.show({ text: `已从 ${name} 移除` })
    } else {
      await memberWrites.append(id, [rj])
      toast.show({ text: `已添加到 ${name}` })
    }
  } catch (e) { toast.show({ text: e instanceof Error ? e.message : String(e) }) }
}

async function addToNewCollection(): Promise<void> {
  const rj = store.rj
  if (locked.value || rj === null) return
  try {
    const made = await manage.create(newCollectionName(collections.value))
    await memberWrites.append(made.id, [rj])
    toast.show({ text: `已新建 ${made.name} 并添加` })
  } catch (e) { toast.show({ text: e instanceof Error ? e.message : String(e) }) }
}

function saveNote(event: Event): void {
  if (locked.value) return
  const value = (event.target as HTMLTextAreaElement).value
  if (work.value && value !== work.value.note) userData.mutate({ note: value })
}

// ── My tags: pick from the vocabulary, or create a tag by name (idempotent). ──
const adding = ref(false)
const newTag = ref('')
const addable = computed(() => allTags.value.filter((tag) => !mine.value.some((entry) => entry.id === tag.id)))

function addTag(id: string): void {
  adding.value = false
  tagReplace.mutate([...mine.value.map((tag) => tag.id), id])
}

function removeTag(id: string): void {
  tagReplace.mutate(mine.value.filter((tag) => tag.id !== id).map((tag) => tag.id))
}

async function createTag(): Promise<void> {
  const name = newTag.value.trim()
  if (name === '') return
  const tag = await tagCreate.mutateAsync(name)
  newTag.value = ''
  addTag(tag.id)
}

/** A catalog date or timestamp, as `YYYY.MM.DD`. */
function dotted(value: string | null): string {
  return dottedDate(value === null ? null : value.slice(0, 10))
}

/** A copy's online square: the manifest knows, the catalog listing does not. */
function copyOnline(copyId: string): boolean | null {
  return store.content?.copies.find((copy) => copy.copy_id === copyId)?.online ?? null
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
    nodes: [slot.root, ...(props.compact ? slot.translations.filter((node) => node.rj === self) : slot.translations)],
  }))
})

/** A copy's library link: its folder, or the library overview when offline. */
function copyTo(copy: { copy_id: string; library_id: string; dir_id: string }) {
  return copyOnline(copy.copy_id) === false || copy.dir_id === ''
    ? { name: 'workbench-overview', params: { libraryId: copy.library_id } }
    : { name: 'overview-files', params: { libraryId: copy.library_id, dirId: copy.dir_id } }
}
</script>

<template>
  <div class="colo" :class="{ compact: props.compact }">
    <div class="colo-head">
      <span class="strip-name lbl">奥付<span class="jp"> · COLOPHON</span></span>
      <span class="lbl">{{ work?.rj ?? '—' }}</span>
    </div>
    <h2 class="colo-title" :lang="work?.relation?.language === 'CHI_HANS' ? 'zh-Hans' : work?.relation?.language === 'CHI_HANT' ? 'zh-Hant' : 'ja'">{{ work?.title ?? '—' }}</h2>

    <dl class="colo-box">
      <div class="colo-row"><dt>作品</dt><dd><span class="mono">{{ work?.rj ?? '—' }}</span></dd></div>
      <div class="colo-row"><dt>社团</dt><dd><span lang="ja">{{ work?.circle ?? '—' }}</span></dd></div>
      <div class="colo-row"><dt>声优</dt><dd><span lang="ja">{{ voice }}</span></dd></div>
      <div class="colo-row"><dt>发售</dt>
        <dd><span class="mono">{{ dottedDate(work?.release_date ?? null) }}</span>
          <template v-if="work?.dl_count !== null && work?.dl_count !== undefined"><span class="sep">·</span><span class="lbl">DL<b>{{ work.dl_count.toLocaleString() }}</b></span></template>
        </dd>
      </div>
      <div v-if="age" class="colo-row"><dt>分级</dt>
        <dd><span class="mono">{{ age }}</span></dd>
      </div>
      <div v-if="work?.series_name" class="colo-row"><dt>系列</dt><dd><span lang="ja">{{ work.series_name }}</span></dd></div>
      <div v-if="editionLines.length" class="colo-row"><dt>版本</dt>
        <dd><span class="ed">
          <span v-for="line in editionLines" :key="line.slot.language" class="ed-line"
            :aria-current="line.current ? 'true' : undefined">
            <span class="lbl">{{ SLOT_NAMES[line.slot.language] ?? line.slot.language }}</span>
            <template v-for="(node, index) in line.nodes" :key="node.rj">
              <span v-if="index > 0" class="sep">{{ index === 1 ? '→' : '·' }}</span>
              <span class="tag" :aria-current="node.rj === work?.rj ? 'true' : undefined">
                {{ ROLE_NAMES[node.role] }}
                <RouterLink v-if="node.held && node.rj !== work?.rj" class="mono" :to="{ name: 'work-detail', params: { rj: node.rj } }">{{ node.rj }}</RouterLink>
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
        <dd><span class="tags"><span v-for="tag in work.source_tags" :key="tag.id" class="tag">{{ tag.name }}</span></span></dd>
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

    <div class="colo-head"><span class="lbl">PERSONAL</span></div>
    <dl class="colo-box">
      <div class="colo-row"><dt>评分</dt>
        <dd>
          <span class="rating">
            <i v-for="n in 10" :key="n" class="on-able" :class="{ on: rating !== null && n <= rating }"
              :role="locked ? undefined : 'button'" :tabindex="locked ? undefined : 0"
              :aria-label="`评分 ${n}`" @click="setRating(n)" @keydown.enter="setRating(n)"></i>
          </span>
          <span class="lbl"><b>{{ rating ?? '—' }}</b>/10</span>
          <button class="fav" type="button" :aria-pressed="favorite" :disabled="locked" @click="toggleFavorite">♥ FAV</button>
          <DropdownMenu>
            <DropdownMenuTrigger as-child>
              <button class="fav" type="button" :disabled="locked" data-testid="work-collections">+ 合集</button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" class="w-52" data-own-keys>
              <DropdownMenuCheckboxItem v-for="c in userCollections" :key="c.id" :model-value="held.includes(c.id)"
                data-testid="work-collection-item" :data-id="c.id" @select.prevent="toggleCollection(c.id, c.name)">{{ c.name }}</DropdownMenuCheckboxItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem data-testid="work-collection-new" @select="addToNewCollection">新建合集</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </dd>
      </div>
      <div class="colo-row"><dt>我的</dt>
        <dd>
          <span class="tags">
            <button v-for="tag in mine" :key="tag.id" class="tag mine" type="button" :disabled="locked"
              :title="locked ? undefined : '移除这个标签'" @click="removeTag(tag.id)">{{ tag.name }}</button>
            <button class="tag add" type="button" :disabled="locked" :aria-expanded="adding"
              @click="adding = !adding">+ TAG</button>
          </span>
          <span v-if="adding" class="menu" role="menu">
            <div class="strip"><span class="strip-name lbl">ADD TAG</span></div>
            <button v-for="tag in addable" :key="tag.id" class="menu-item" type="button" role="menuitem"
              @click="addTag(tag.id)"><span class="glyph">¶</span>{{ tag.name }}</button>
            <form class="menu-item new-tag" @submit.prevent="createTag">
              <span class="glyph">＋</span>
              <input v-model="newTag" data-own-keys type="text" placeholder="NEW TAG" />
            </form>
          </span>
        </dd>
      </div>
      <div class="colo-row"><dt>笔记</dt>
        <dd><textarea class="note" :rows="props.compact ? 2 : 3" :value="work?.note ?? ''" :disabled="locked"
          @change="saveNote"></textarea></dd></div>
    </dl>

    <div class="colo-foot">
      <span class="lbl">ONSEI ARCHIVE</span>
      <span class="lbl">{{ props.compact ? 'FLIP BACK ◢' : '' }}</span>
    </div>
  </div>
</template>

<style scoped>
.colo { padding: var(--sp-5) var(--sp-5) var(--sp-6); display: flex; flex-direction: column; gap: var(--sp-4); }
.colo.compact { padding: var(--sp-4); gap: var(--sp-3); }
.colo.compact .colo-title { font-size: var(--fs-m); line-height: var(--lh-m); }
.colo-head { display: flex; justify-content: space-between; align-items: baseline; gap: var(--sp-3); }
.colo-head .strip-name::before { display: none; }
.strip-name .jp { font-family: var(--font-ui); letter-spacing: 0; }
.colo-title { font-family: var(--font-mincho); font-size: var(--fs-l); line-height: var(--lh-l); font-weight: 700; letter-spacing: 0; }
.colo-box { border-top: 2px solid var(--wire-strong); border-bottom: 1px solid var(--wire-strong); }
.colo-row { display: grid; grid-template-columns: 72px 1fr; gap: var(--sp-3); align-items: baseline; padding: 7px 0 6px; border-bottom: 1px solid var(--wire-fine); }
.colo-row:last-child { border-bottom: none; }
.colo-row dt { font-size: var(--fs-s); color: var(--fg-muted); letter-spacing: .3em; white-space: nowrap; }
.colo-row dd { font-size: var(--fs-s); font-weight: 500; min-width: 0; position: relative; }
/* The Japanese face for lang="ja" values ( matrix untouched);
   mono telemetry keeps its own face. */
.colo dd :lang(ja):not(.mono) { font-family: var(--font-ja); }
.mono { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .06em; }
.colo-row dd a { color: inherit; text-decoration: underline; text-decoration-color: var(--wire-mid); text-underline-offset: 3px; font-family: var(--font-mono); font-size: var(--fs-xs); word-break: break-all; }
.tags { display: flex; flex-wrap: wrap; gap: 4px; }
.tag { height: 18px; padding: 0 6px; display: inline-flex; align-items: center; border: 1px solid var(--wire-fine); font-size: var(--fs-xs); }
.tag[aria-current='true'] { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }
.tag.mine, .tag.add { border-color: var(--wire-mid); }
.tag.mine:hover { border-color: var(--wire-strong); }
.tag.add { border-style: dashed; color: var(--fg-meta); font-family: var(--font-mono); }
.ed { display: flex; flex-direction: column; gap: 4px; }
.ed-line { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; }
.ed-line[aria-current='true'] > .lbl { color: var(--fg-base); font-weight: 700; }
.tag .lbl { margin-left: 4px; }
.tag a { color: inherit; text-decoration: underline; text-decoration-color: var(--wire-mid); text-underline-offset: 3px; margin-left: 4px; }
.copy { display: grid; grid-template-columns: auto 1fr; gap: 2px var(--sp-2); }
.copy > span:nth-child(even) { min-width: 0; overflow-wrap: anywhere; }
.rating { display: inline-flex; gap: 3px; vertical-align: middle; }
.rating i { width: 10px; height: 10px; border: 1px solid var(--wire-mid); }
.rating i.on { background: var(--invert-bg); border-color: var(--invert-bg); }
.rating i[role='button'] { cursor: pointer; }
.rating i[role='button']:hover { border-color: var(--wire-strong); }
.fav { font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .12em; height: 18px; padding: 0 6px; border: 1px solid var(--wire-mid); margin-left: var(--sp-3); color: var(--fg-base); background: none; cursor: pointer; }
.fav[aria-pressed='true'] { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }
.note { width: 100%; min-height: 64px; resize: vertical; background: transparent; border: 1px solid var(--wire-fine); padding: var(--sp-2); font-size: var(--fs-s); line-height: 20px; color: var(--fg-base); outline: none; }
.note:focus { border-color: var(--wire-strong); }
.note:disabled { opacity: .5; }
.colo-foot { display: flex; justify-content: space-between; }
/* The + TAG specimen panel, same grammar as the leaf's pair menu. */
.menu { position: absolute; right: 0; top: calc(100% + 8px); z-index: 20; width: 264px; padding-bottom: var(--sp-2); background: var(--grain), var(--bg-panel); border: 1px solid var(--wire-strong); box-shadow: 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid); }
.menu .strip { margin: var(--sp-2) var(--sp-2) var(--sp-1); }
.menu-item { display: flex; align-items: center; gap: var(--sp-2); width: calc(100% - 16px); margin: 0 var(--sp-2); height: 28px; padding: 0 var(--sp-2) 0 var(--sp-1); border-bottom: 1px solid var(--wire-fine); font-size: var(--fs-s); text-align: left; color: var(--fg-base); background: none; cursor: pointer; }
.menu-item:hover { background: var(--invert-bg); color: var(--invert-fg); }
.menu-item .glyph { width: 14px; text-align: center; font-family: var(--font-mono); font-size: var(--fs-xs); color: inherit; }
.menu-item.new-tag { border-bottom: none; }
.menu-item input { flex: 1; min-width: 0; border: none; border-bottom: 1px solid var(--wire-mid); background: transparent; color: inherit; font-size: var(--fs-s); outline: none; }
</style>
