<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import GlobalMenu from '@/components/layout/GlobalMenu.vue'
import CollectionList from '@/features/collections/CollectionList.vue'
import PlayerBar from '@/features/works/PlayerBar.vue'
import RailSelect, { type RailOption } from '@/features/works/RailSelect.vue'
import type { WorksSort } from '@/lib/api/types'
import { useCollectionPages } from '@/queries/collections'

// One collection's detail page: header, the ordered list, the player baseline.
// The view sort is the address's (`sort`, `direction`; no query is the saved
// order) and never touches the saved order. The header's right side is kept
// for the collection's rename / delete actions, which come later.

const route = useRoute()
const router = useRouter()
const id = computed(() => String(route.params.id))

// Dates and ratings read newest / highest first; the saved order and the work number, from the start.
const SORT_OPTIONS: RailOption[] = [
  { value: '', label: '自定义顺序' },
  { value: 'rj', label: '作品编号' },
  { value: 'release_date', label: '发售时间' },
  { value: 'collected_at', label: '添加时间' },
  { value: 'rating', label: '个人评分' },
]
const sort = computed(() => SORT_OPTIONS.find((o) => o.value === route.query.sort)?.value as WorksSort | '' ?? '')
const defaultDirection = (s: string) => (s === '' ? 'asc' : 'desc')
const direction = computed(() => (route.query.direction === 'asc' || route.query.direction === 'desc' ? route.query.direction : defaultDirection(sort.value)))

function show(nextSort: string, nextDirection: string) {
  void router.replace({ query: {
    ...(nextSort ? { sort: nextSort } : {}),
    ...(nextDirection !== defaultDirection(nextSort) ? { direction: nextDirection } : {}),
  } })
}
const sortModel = computed({ get: () => sort.value, set: (s: string) => show(s, defaultDirection(s)) })

const { query, name, total, works } = useCollectionPages(id, sort, direction)

// Back to the entry this collection was opened from: that history entry holds
// the selection to restore. Arriving any other way, the entry opens fresh.
function backToEntry(): void {
  if (router.options.history.state.back === '/collections') router.back()
  else void router.push({ name: 'collections' })
}
</script>

<template>
  <section class="detail substrate" aria-label="合集详情" data-testid="collection-detail-page">
    <header class="header">
      <div class="header-left">
        <GlobalMenu />
        <button class="btn btn-s" type="button" data-testid="collection-back" @click="backToEntry">
          <svg viewBox="0 0 10 10" aria-hidden="true" style="transform: scaleX(-1)"><path d="M0 4.5h7.2L4.6 1.9l.7-.7L9.1 5 5.3 8.8l-.7-.7 2.6-2.6H0z" /></svg>BACK
        </button>
        <h1 class="h-title">{{ name }}</h1>
        <span v-if="query.data.value" class="lbl">WORKS <b>{{ total }}</b></span>
      </div>
      <div class="header-meta">
        <RailSelect v-model="sortModel" label="SORT" :options="SORT_OPTIONS" />
        <button class="btn btn-s" type="button" :title="direction === 'asc' ? '升序' : '降序'" data-testid="collection-direction"
          @click="show(sort, direction === 'asc' ? 'desc' : 'asc')">{{ direction === 'asc' ? '↑ ASC' : '↓ DESC' }}</button>
        <span class="actions" data-testid="collection-actions" />
      </div>
    </header>

    <main class="main">
      <CollectionList v-if="total > 0" :works="works" :total="total" :custom="sort === ''" :direction="direction"
        :has-more="query.hasNextPage.value" @more="!query.isFetchingNextPage.value && query.fetchNextPage()" />
      <p v-else-if="query.data.value" class="empty" data-testid="collection-empty">
        这个合集还是空的。<RouterLink :to="{ name: 'works' }">去作品墙挑选</RouterLink>
      </p>
    </main>

    <PlayerBar />
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
.empty { padding: var(--sp-6); font-size: var(--fs-m); }
.empty a { margin-left: var(--sp-2); text-decoration: underline; text-underline-offset: 4px; }
@media (max-width: 900px) {
  .header { padding: 0 var(--sp-4); }
  .header::after { left: var(--sp-4); right: var(--sp-4); }
  .header .lbl { display: none; }
}
</style>
