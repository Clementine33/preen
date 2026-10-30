<script setup lang="ts">
import type { Virtualizer } from '@tanstack/vue-virtual'
import type { MemberFilter } from '@/app/route-params'
import type { EditTarget } from '@/features/worksets/draft-intents'
import type { MemberConclusion } from '@/features/worksets/plan-readers'
import type { OperationDraftDocument, WorksetMember } from '@/lib/api/types'
import { Eye, FolderOpen, ListChecks } from '@lucide/vue'
import { observeElementOffset, useVirtualizer } from '@tanstack/vue-virtual'
import { computed, ref } from 'vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { OVERRIDE_UNITS, readUnit } from '@/features/worksets/draft-intents'

const props = defineProps<{
  members: WorksetMember[]
  draft: OperationDraftDocument
  selectedIds: Set<string>
  hiddenSelectedCount: number
  filter: MemberFilter
  search: string
  conclusionFor: (member: WorksetMember) => MemberConclusion
  /**
   * The current revision's execution scope: `null` when it has no execution
   * session, `[]` for a whole-revision run, else the record-relative paths of
   * the folders it covered. Only members in scope offer 执行结果.
   */
  executionScope?: string[] | null
}>()
const emit = defineEmits<{
  /** Opening a member's files: the shared file module, in this workbench. */
  'files': [memberId: string]
  'toggle': [memberId: string]
  'toggleAll': [ids: string[]]
  'open': [memberId: string]
  'openExecution': [memberId: string]
  'update:filter': [value: MemberFilter]
  'update:search': [value: string]
}>()
/**
 * The operation's member list.
 *
 * Clicking a row body opens the member (view), the checkbox only selects:
 * selection never drills in and viewing never selects. Filtering never
 * clears the selection, and a filter that hides selected rows reports how many
 * are hidden so "修改 N 个文件夹" never counts invisible names.
 *
 * Rows are virtualized because a record can hold thousands of folders. A row
 * wraps its name and path, so its height is measured per row rather than fixed;
 * ROW_ESTIMATE only seeds the scroll extent until a row has been drawn.
 */
const ROW_ESTIMATE = 76
const OVERSCAN = 8

const FILTERS: { value: MemberFilter, label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'change', label: '有变化' },
  { value: 'warn', label: '未满足' },
  { value: 'blocked', label: '阻塞' },
  { value: 'source_conflict', label: '源竞争' },
  { value: 'excluded', label: '已排除' },
]

const scopeSet = computed(() => (props.executionScope?.length
  ? new Set(props.executionScope)
  : null))
function hasExecutionResult(member: WorksetMember): boolean {
  return props.executionScope != null && (scopeSet.value === null || scopeSet.value.has(member.rel_path))
}

const excluded = computed(() => {
  const set = new Set<string>()
  for (const record of props.draft.members) {
    if (record.excluded)
      set.add(record.member_id)
  }
  return set
})

const visible = computed(() =>
  props.members.filter((member) => {
    const query = props.search.trim().toLowerCase()
    if (query && !member.folder_name.toLowerCase().includes(query) && !member.rel_path.toLowerCase().includes(query)) {
      return false
    }
    switch (props.filter) {
      case 'excluded':
        return excluded.value.has(member.member_id)
      case 'source_conflict':
      case 'change':
      case 'warn':
      case 'blocked':
        return props.conclusionFor(member).filter === props.filter
      default:
        return true
    }
  }),
)

const scrollEl = ref<HTMLElement | null>(null)
const virtualizer = useVirtualizer(
  computed(() => ({
    count: visible.value.length,
    getScrollElement: () => scrollEl.value,
    estimateSize: () => ROW_ESTIMATE,
    overscan: OVERSCAN,
    // A mounted list is hidden behind narrow member details. The library's
    // scroll-settle timer can then read scrollTop=0 from display:none, losing
    // the return position. Keep the last visible offset while it has no layout.
    observeElementOffset: (instance: Virtualizer<HTMLElement, Element>, report) => observeElementOffset(instance, (offset, scrolling) => {
      if (instance.scrollElement?.offsetHeight)
        report(offset, scrolling)
    }),
    getItemKey: (index: number) => visible.value[index]?.member_id ?? index,
  })),
)

// One stable callback: an inline arrow is a new ref every render, which would
// re-measure on each pass and feed the virtualizer's own updates back in.
function measureRow(el: unknown): void {
  virtualizer.value.measureElement(el as Element | null)
}

const rows = computed(() =>
  virtualizer.value.getVirtualItems().flatMap((row) => {
    const member = visible.value[row.index]
    return member
      ? [
          { row, member },
        ]
      : []
  }),
)

const visibleIds = computed(() => visible.value.map(m => m.member_id))
const allVisibleSelected = computed(
  () => visibleIds.value.length > 0 && visibleIds.value.every(id => props.selectedIds.has(id)),
)

/**
 * Whether a member carries any setting of its own. Which units are overridden
 * is the detail view's job: in the list a single 默认／已修改 state says what
 * the user needs to know without five unreadable per-unit tags.
 */
function hasOverride(member: WorksetMember): boolean {
  return OVERRIDE_UNITS.some((unit) => {
    const value = readUnit(props.draft, { kind: 'member', memberId: member.member_id } as EditTarget, unit)
    return value.source === 'member'
  })
}
</script>

<template>
  <div class="flex min-h-0 flex-col">
    <!-- The toolbar wraps instead of pushing anything out of the container. -->
    <div class="flex shrink-0 flex-col gap-2 border-b border-border px-3 py-2" data-testid="member-toolbar">
      <input
        :value="search"
        type="search"
        placeholder="搜索文件夹"
        aria-label="搜索文件夹"
        class="h-9 w-full rounded-md border border-[var(--control-border)] bg-background px-3 text-sm focus-visible:border-[var(--wire-strong)] focus-visible:outline-none @[641px]:max-w-xs"
        @input="emit('update:search', ($event.target as HTMLInputElement).value)"
      >
      <div class="flex flex-wrap items-center gap-1.5" role="group" aria-label="筛选">
        <Button
          v-for="item in FILTERS"
          :key="item.value"
          :variant="filter === item.value ? 'secondary' : 'ghost'"
          size="sm"
          class="h-9 px-3 text-sm"
          :aria-pressed="filter === item.value"
          @click="emit('update:filter', item.value)"
        >
          {{ item.label }}
        </Button>
      </div>
      <span v-if="hiddenSelectedCount > 0" class="text-xs text-[var(--text-muted)]" data-testid="hidden-selection">
        另有 {{ hiddenSelectedCount }} 个已选文件夹被筛选隐藏
      </span>
    </div>

    <label
      class="flex min-h-11 shrink-0 cursor-pointer items-center gap-3 border-b border-[var(--wire-strong)] bg-card px-3 text-xs text-[var(--text-muted)]"
    >
      <input
        type="checkbox"
        class="size-5 shrink-0"
        aria-label="全选当前筛选结果"
        :checked="allVisibleSelected"
        @change="emit('toggleAll', visibleIds)"
      >
      <span>全选当前筛选结果</span>
      <span class="ml-auto font-mono">{{ visible.length }} / {{ members.length }}</span>
    </label>

    <div ref="scrollEl" class="min-h-0 flex-1 overflow-y-auto overflow-x-hidden" data-testid="member-scroller">
      <ul
        role="list"
        aria-label="工作集成员与转换结论"
        class="relative m-0 w-full list-none p-0"
        :style="{ height: `${virtualizer.getTotalSize()}px` }"
      >
        <li
          v-for="{ row, member } in rows"
          :key="member.member_id"
          :ref="measureRow"
          :data-index="row.index"
          :aria-posinset="row.index + 1"
          :aria-setsize="visible.length"
          :style="{ transform: `translateY(${row.start}px)` }"
          class="absolute top-0 left-0 w-full grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 border-b border-border/60 px-3 py-2.5 hover:bg-accent/40 @[641px]:grid-cols-[auto_minmax(0,1fr)_auto_auto]"
          :class="{ 'bg-accent/30': selectedIds.has(member.member_id) }"
          data-testid="member-row"
        >
          <label class="col-start-1 row-span-2 row-start-1 -m-2 grid size-11 cursor-pointer place-items-center @[641px]:row-span-1">
            <input
              type="checkbox"
              class="size-5"
              :checked="selectedIds.has(member.member_id)"
              :aria-label="`选择 ${member.folder_name}`"
              @change="emit('toggle', member.member_id)"
            >
          </label>

          <div class="col-start-2 row-start-1 min-w-0">
            <button
              type="button"
              class="block w-full text-left text-sm font-medium leading-5 hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              :title="member.folder_path"
              data-testid="member-open"
              @click="emit('open', member.member_id)"
            >
              <span class="line-clamp-3 [overflow-wrap:anywhere]">{{ member.folder_name }}</span>
            </button>
            <span
              v-if="member.rel_path.lastIndexOf('/') > 0"
              class="mt-0.5 block truncate font-mono text-[11px] leading-4 text-[var(--text-muted)]"
              :title="member.folder_path"
            >
              {{ member.rel_path.slice(0, member.rel_path.lastIndexOf('/')) }}
            </span>
          </div>

          <div
            class="col-start-2 row-start-2 flex flex-wrap items-center gap-1.5 @[641px]:col-start-3 @[641px]:row-start-1"
          >
            <Badge :tone="conclusionFor(member).tone" :title="conclusionFor(member).detail">
              {{ conclusionFor(member).label }}
            </Badge>
            <!-- Only the exception is marked: a folder on the defaults carries no chip. -->
            <Badge v-if="hasOverride(member)" tone="brand" data-testid="member-override-state">已修改</Badge>
          </div>

          <!-- The member's files: the shared file module, opened inside this
               workbench (ADR 0001). -->
          <div
            class="col-start-3 row-span-2 row-start-1 flex items-center gap-1 @[641px]:col-start-4 @[641px]:row-span-1"
          >
            <Button
              variant="ghost"
              size="icon"
              class="size-10"
              :aria-label="`打开 ${member.folder_name} 的文件`"
              title="当前文件"
              data-testid="member-files"
              @click="emit('files', member.member_id)"
            >
              <FolderOpen class="size-5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              class="size-10"
              :aria-label="`查看 ${member.folder_name}`"
              title="查看"
              data-testid="member-open-icon"
              @click="emit('open', member.member_id)"
            >
              <Eye class="size-5" />
            </Button>
            <Button
              v-if="hasExecutionResult(member)"
              variant="ghost"
              size="icon"
              class="size-10"
              :aria-label="`查看 ${member.folder_name} 的执行结果`"
              title="执行结果"
              data-testid="member-execution"
              @click="emit('openExecution', member.member_id)"
            >
              <ListChecks class="size-5" />
            </Button>
          </div>
        </li>
      </ul>
      <p v-if="visible.length === 0" class="px-3 py-8 text-center text-xs text-[var(--text-muted)]">
        没有符合当前筛选的文件夹。
      </p>
    </div>
  </div>
</template>
