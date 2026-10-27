<script setup lang="ts">
import type { LibraryDir } from '@/lib/api/types'
import { ChevronRight, Folder } from '@lucide/vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { computed, ref } from 'vue'

const props = defineProps<{
  dirs: LibraryDir[]
  /** The selected library-relative paths: the identity a record is created from. */
  selectedPaths: string[]
  allSelected: boolean
}>()
const emit = defineEmits<{
  select: [relPath: string, selected: boolean]
  selectAll: [selected: boolean]
  /** Opening a directory: the identity its page is addressed by. */
  open: [dirId: string]
  /** The list's scroll offset: the caller keeps it across a trip away. */
  scroll: [offset: number]
}>()
/**
 * The overview's directory list (ADR 0001).
 *
 * Every scanned work root in the library, empty ones and ones
 * without audio included. Rows are virtualized because a library root can
 * hold hundreds of folders.
 *
 * Clicking a name selects the row; only the chevron button opens the folder's
 * files. A nested folder shows its parent path under the name, as the member
 * list does. The grid is checkbox, name, open at every tier and its width
 * classes never force a minimum, so nothing hides behind a horizontal overflow.
 *
 * Fixed row height is load-bearing for the virtualizer's spacer math and is the
 * SAME in both tiers, so crossing a breakpoint never invalidates a measured
 * offset. Every cell is single-line/truncated, so no row can exceed h-14; if a
 * future cell starts wrapping, update this constant and the row class together.
 */
const ROW_HEIGHT = 56
const OVERSCAN = 10

/** The caller owns the offset: hiding this list collapses the element's own. */
defineExpose({
  getScrollTop: (): number => scrollEl.value?.scrollTop ?? 0,
  setScrollTop: (offset: number): void => {
    if (scrollEl.value)
      scrollEl.value.scrollTop = offset
  },
})

const scrollEl = ref<HTMLElement | null>(null)

// @tanstack/vue-virtual owns the scroll observation (it attaches its own
// scroll/resize listeners once getScrollElement resolves) and re-renders
// reactively after each scroll; the window is a slice of the same single data
// source in every layout (no second mobile list).
const virtualizer = useVirtualizer(
  computed(() => ({
    count: props.dirs.length,
    getScrollElement: () => scrollEl.value,
    estimateSize: () => ROW_HEIGHT,
    overscan: OVERSCAN,
  })),
)

function checkboxValue(event: Event): boolean {
  return (event.target as HTMLInputElement).checked
}

function parentOf(relPath: string): string {
  const slash = relPath.lastIndexOf('/')
  return slash > 0
    ? relPath.slice(0, slash)
    : ''
}

function dirAt(index: number): LibraryDir {
  return props.dirs[index]
}
</script>

<template>
  <div class="flex h-full min-w-0 flex-col" data-testid="dir-list">
    <!-- Desktop keeps the column labels; the phone tier shows the count and the
         select-all control instead, because the rows are merged there. -->
    <div
      class="grid h-9 shrink-0 grid-cols-[44px_minmax(0,1fr)_44px] items-center border-t border-border border-b-[var(--wire-strong)] border-b bg-muted/35 px-2 text-[11px] font-medium text-muted-foreground rail:grid-cols-[38px_minmax(0,1fr)_92px] rail:px-3"
    >
      <label class="flex h-full w-full cursor-pointer items-center justify-center rail:justify-start">
        <input
          data-testid="select-all-dirs"
          type="checkbox"
          :checked="allSelected"
          aria-label="选择全部文件夹"
          class="size-4 accent-[var(--ring)] rail:size-3.5"
          @change="emit('selectAll', checkboxValue($event))"
        >
      </label>
      <div class="min-w-0 truncate">
        <span class="hidden rail:inline">文件夹</span>
        <span class="rail:hidden" data-testid="dir-count">共 {{ dirs.length }} 个文件夹</span>
      </div>
      <div class="hidden rail:block" />
    </div>

    <div
      ref="scrollEl"
      data-testid="dir-list-scroller"
      class="min-h-0 flex-1 overflow-y-auto"
      @scroll="emit('scroll', ($event.target as HTMLElement).scrollTop)"
    >
      <ul
        data-testid="dir-list-spacer"
        class="relative m-0 w-full list-none p-0"
        :style="{ height: `${virtualizer.getTotalSize()}px` }"
      >
        <li
          v-for="virtualItem in virtualizer.getVirtualItems()"
          :key="dirAt(virtualItem.index).rel_path"
          :style="{ transform: `translateY(${virtualItem.start}px)` }"
          class="absolute top-0 left-0 grid h-14 w-full grid-cols-[44px_minmax(0,1fr)_44px] items-center overflow-hidden border-b border-border px-2 transition-colors hover:bg-accent/45 rail:grid-cols-[38px_minmax(0,1fr)_92px] rail:px-3"
        >
          <label class="flex h-full w-full cursor-pointer items-center justify-center rail:justify-start">
            <input
              :id="`dir-select-${dirAt(virtualItem.index).dir_id}`"
              :data-testid="`dir-checkbox-${dirAt(virtualItem.index).rel_path}`"
              type="checkbox"
              :checked="selectedPaths.includes(dirAt(virtualItem.index).rel_path)"
              :aria-label="`选择 ${dirAt(virtualItem.index).name}`"
              class="size-4 accent-[var(--ring)] rail:size-3.5"
              @change="emit('select', dirAt(virtualItem.index).rel_path, checkboxValue($event))"
            >
          </label>
          <label
            :for="`dir-select-${dirAt(virtualItem.index).dir_id}`"
            :data-testid="`dir-name-${dirAt(virtualItem.index).rel_path}`"
            class="flex h-full min-w-0 cursor-pointer items-center gap-2"
          >
            <Folder class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span class="min-w-0 flex-1">
              <span
                class="block truncate font-heading text-sm font-semibold"
                :title="dirAt(virtualItem.index).name"
              >
                {{ dirAt(virtualItem.index).name }}
              </span>
              <span
                v-if="parentOf(dirAt(virtualItem.index).rel_path)"
                class="block truncate font-mono text-[11px] text-muted-foreground"
                :title="parentOf(dirAt(virtualItem.index).rel_path)"
              >
                {{ parentOf(dirAt(virtualItem.index).rel_path) }}
              </span>
            </span>
          </label>
          <!-- Always on screen, never hover-only; the phone tier keeps a
               44px target while the desktop tier stays compact. -->
          <button
            type="button"
            class="flex h-full w-full items-center justify-center rounded text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none rail:h-8 rail:w-8"
            :aria-label="`打开 ${dirAt(virtualItem.index).name}`"
            @click="emit('open', dirAt(virtualItem.index).dir_id)"
          >
            <ChevronRight class="size-4" aria-hidden="true" />
          </button>
        </li>
      </ul>
    </div>
  </div>
</template>
