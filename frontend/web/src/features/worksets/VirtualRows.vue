<script setup lang="ts" generic="T">
import { computed, ref } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'

/**
 * A virtualized single-line list for the task pages, where a record can hold
 * thousands of folders. Fixed row height is load-bearing for the virtualizer's
 * spacer math: a row's content must stay single-line/truncated, so keep
 * ROW_HEIGHT and the row's `h-8` together.
 */
const ROW_HEIGHT = 32
const OVERSCAN = 10

const props = defineProps<{
  items: T[]
  itemKey: (item: T) => string
  testid: string
}>()
defineSlots<{ default(props: { item: T }): unknown }>()

const scrollEl = ref<HTMLElement | null>(null)
const virtualizer = useVirtualizer(
  computed(() => ({
    count: props.items.length,
    getScrollElement: () => scrollEl.value,
    estimateSize: () => ROW_HEIGHT,
    overscan: OVERSCAN,
  })),
)
</script>

<template>
  <div ref="scrollEl" class="max-h-[60vh] overflow-y-auto text-xs" :data-testid="`${testid}-scroller`">
    <ul class="relative m-0 w-full list-none p-0" :data-testid="testid" :style="{ height: `${virtualizer.getTotalSize()}px` }">
      <li
        v-for="row in virtualizer.getVirtualItems()"
        :key="itemKey(items[row.index])"
        :style="{ transform: `translateY(${row.start}px)` }"
        class="absolute top-0 left-0 grid h-8 w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 border-b border-border"
      >
        <slot :item="items[row.index]" />
      </li>
    </ul>
  </div>
</template>
