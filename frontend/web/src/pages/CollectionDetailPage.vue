<script setup lang="ts">
import type { RailOption } from '@/features/works/RailSelect.vue'
import type { WorksSort } from '@/lib/api/types'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import Modal from '@/components/ui/modal/Modal.vue'
import { useToast } from '@/composables/use-toast'
import CollectionList from '@/features/collections/CollectionList.vue'
import { useCollectionEdit } from '@/features/collections/use-collection-edit'
import PlayerBar from '@/features/works/PlayerBar.vue'
import RailSelect from '@/features/works/RailSelect.vue'
import { useCollectionManage, useCollectionPages } from '@/queries/collections'

// One collection's detail page: header, the ordered list, the player baseline.
// The view sort is the address's (`sort`, `direction`; no query is the saved
// order) and never touches the saved order. EDIT switches the list into its
// edit mode, where members are moved and removed (through useCollectionEdit)
// and nothing plays or opens; the mode is the page's, not the address's. The
// header's `⋯` (not for Favorites) renames or deletes the collection through a
// dialog; deleting returns to the entry.

const route = useRoute()
const router = useRouter()
const id = computed(() => String(route.params.id))

// Dates and ratings read newest / highest first; the saved order and the work number, from the start.
const SORT_OPTIONS: RailOption[] = [
  { value: '', label: '自定义顺序' },
  { value: 'rj', label: '作品编号' },
  { value: 'release_date', label: '发售时间' },
  { value: 'collected_at', label: '入库时间' },
  { value: 'rating', label: '个人评分' },
]
const sort = computed(() => SORT_OPTIONS.find(o => o.value === route.query.sort)?.value as WorksSort | '' ?? '')
function defaultDirection(s: string) {
  return s === ''
    ? 'asc'
    : 'desc'
}
const direction = computed(() => (route.query.direction === 'asc' || route.query.direction === 'desc'
  ? route.query.direction
  : defaultDirection(sort.value)))

function show(nextSort: string, nextDirection: string) {
  void router.replace({
    query: {
      ...(nextSort
        ? { sort: nextSort }
        : {}),
      ...(nextDirection !== defaultDirection(nextSort)
        ? { direction: nextDirection }
        : {}),
    },
  })
}
const sortModel = computed({
  get: () => sort.value,
  set: (s: string) => show(s, defaultDirection(s)),
})

const { query, name, total, works } = useCollectionPages(id, sort, direction)
const editing = ref(false)
watch(id, () => { editing.value = false })
const edit = useCollectionEdit({
  id,
  sort,
  direction,
  name,
  works,
  hasMore: () => query.hasNextPage.value,
})

const manage = useCollectionManage()
const toast = useToast()
const dialog = ref<'none' | 'rename' | 'delete'>('none')
const draft = ref('')
const trimmed = computed(() => draft.value.trim())
function openRename() { draft.value = name.value; dialog.value = 'rename' }
async function submitRename() {
  if (!trimmed.value)
    return
  try { await manage.rename(id.value, trimmed.value); dialog.value = 'none' }
  catch (e) { toast.error(e) }
}
async function submitDelete() {
  const gone = name.value
  try {
    await manage.remove(id.value)
    dialog.value = 'none'
    toast.show({ text: `已删除 ${gone}` })
    await router.push({ name: 'collections' })
  }
  catch (e) { toast.error(e) }
}

// Back to the entry this collection was opened from: that history entry holds
// the selection to restore. Arriving any other way, the entry opens fresh.
function backToEntry(): void {
  if (router.options.history.state.back === '/collections')
    router.back()
  else void router.push({ name: 'collections' })
}
</script>

<template>
  <section class="detail substrate" aria-label="合集详情" data-testid="collection-detail-page">
    <header class="header">
      <div class="header-left">
        <GlobalMenu />
        <button class="btn btn-s" type="button" data-testid="collection-back" @click="backToEntry">
          <svg viewBox="0 0 10 10" aria-hidden="true" style="transform: scaleX(-1)"><path d="M0 4.5h7.2L4.6 1.9l.7-.7L9.1 5 5.3 8.8l-.7-.7 2.6-2.6H0z" /></svg><span class="word">BACK</span>
        </button>
        <h1 class="h-title">{{ name }}</h1>
        <DropdownMenu v-if="id !== 'favorites'">
          <DropdownMenuTrigger as-child>
            <button class="btn btn-s more" type="button" aria-label="合集操作" title="合集操作" data-testid="collection-menu">⋯</button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" class="w-36" data-own-keys>
            <DropdownMenuItem data-testid="collection-rename" @select="openRename">重命名</DropdownMenuItem>
            <DropdownMenuItem variant="destructive" data-testid="collection-delete" @select="dialog = 'delete'">删除合集</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div class="header-meta">
        <RailSelect v-model="sortModel" label="SORT" :options="SORT_OPTIONS" />
        <button
          class="btn btn-s" type="button" :title="direction === 'asc' ? '升序' : '降序'" data-testid="collection-direction"
          @click="show(sort, direction === 'asc' ? 'desc' : 'asc')"
        >{{ direction === 'asc' ? '↑' : '↓' }}<span class="word">{{ direction === 'asc' ? ' ASC' : ' DESC' }}</span></button>
        <span class="actions" data-testid="collection-actions">
          <button
            v-if="total > 0" class="btn btn-s" :class="{ on: editing }" type="button" :aria-pressed="editing"
            data-testid="collection-edit" @click="editing = !editing"
          >{{ editing ? 'DONE' : 'EDIT' }}</button>
        </span>
      </div>
    </header>

    <main class="main">
      <CollectionList
        v-if="total > 0" :works="works" :total="total" :custom="sort === ''" :direction="direction" :editing="editing" :has-more="query.hasNextPage.value" @move="edit.move" @remove="edit.remove"
        @done="editing = false" @more="!query.isFetchingNextPage.value && query.fetchNextPage()"
      />
      <p v-else-if="query.data.value" class="empty" data-testid="collection-empty">
        这个合集还是空的。<RouterLink :to="{ name: 'works' }">去作品墙挑选</RouterLink>
      </p>
    </main>

    <PlayerBar />

    <Modal :open="dialog === 'rename'" title="重命名合集" @update:open="dialog = 'none'">
      <form class="grid gap-3" @submit.prevent="submitRename">
        <input
          v-model="draft" class="h-9 w-full rounded-md border border-input bg-transparent px-2 text-xs focus-visible:border-[var(--wire-strong)] focus-visible:outline-none"
          aria-label="合集名称" data-testid="collection-rename-input"
        >
        <p v-if="!trimmed" class="text-[11px] text-[var(--text-muted)]">名称不能为空。</p>
        <div class="flex justify-end gap-2">
          <Button type="button" variant="ghost" size="sm" @click="dialog = 'none'">取消</Button>
          <Button type="submit" size="sm" :disabled="!trimmed" data-testid="collection-rename-submit">保存</Button>
        </div>
      </form>
    </Modal>
    <Modal :open="dialog === 'delete'" title="删除合集" @update:open="dialog = 'none'">
      <p class="text-xs">删除合集「{{ name }}」？其中 {{ total }} 部作品与文件不受影响。</p>
      <div class="flex justify-end gap-2">
        <Button variant="ghost" size="sm" @click="dialog = 'none'">取消</Button>
        <Button variant="destructive" size="sm" data-testid="collection-delete-confirm" @click="submitDelete">删除</Button>
      </div>
    </Modal>
  </section>
</template>

<style scoped>
.detail { display: grid; grid-template-rows: 48px minmax(0, 1fr) auto; width: 100%; height: 100%; font-variant-numeric: tabular-nums; }
.header { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-4); padding: 0 var(--sp-6); position: relative; min-width: 0; }
.header::after { content: ''; position: absolute; left: var(--sp-6); right: var(--sp-6); bottom: 0; height: 4px; border-top: 1px solid var(--wire-fine); border-bottom: 2px solid var(--wire-strong); }
.header-left { display: flex; align-items: center; gap: var(--sp-2); min-width: 0; }
.header-meta { display: flex; align-items: center; gap: var(--sp-2); flex-shrink: 0; }
.h-title { font-size: var(--fs-m); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.main { display: grid; min-height: 0; min-width: 0; }
.actions { display: inline-flex; gap: var(--sp-2); }
.more { width: 28px; padding: 0; justify-content: center; }
.actions .btn.on { background: var(--invert-bg); color: var(--invert-fg); }
.empty { padding: var(--sp-6); font-size: var(--fs-m); }
.empty a { margin-left: var(--sp-2); text-decoration: underline; text-underline-offset: 4px; }
@media (max-width: 900px) {
  .header { padding: 0 var(--sp-4); }
  .header::after { left: var(--sp-4); right: var(--sp-4); }
  .header-meta { gap: var(--sp-1); }
  .header-meta :deep(.rs-lbl) { display: none; }
  /* A phone keeps the title readable: BACK and the direction keep only their arrows. */
  .word { display: none; }
}
</style>
