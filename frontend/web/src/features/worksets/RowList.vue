<script setup lang="ts" generic="T">
/**
 * A single-line list for the task pages, where a record can hold thousands of
 * folders. Every row is rendered, but `content-visibility: auto` makes the
 * browser skip layout and paint of the rows out of view; `auto 32px` reserves
 * the row height until it has been drawn once, so the scrollbar stays put. The
 * rows are plain DOM, so find-in-page and assistive tech still reach all of
 * them. Keep the 32px and the row's `h-8` together.
 */
defineProps<{
  items: T[]
  itemKey: (item: T) => string
  testid: string
}>()
defineSlots<{ default(props: { item: T }): unknown }>()
</script>

<template>
  <div class="max-h-[60vh] overflow-y-auto text-xs" :data-testid="`${testid}-scroller`">
    <ul class="m-0 w-full list-none p-0" :data-testid="testid">
      <li
        v-for="item in items"
        :key="itemKey(item)"
        class="grid h-8 w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 border-b border-border [contain-intrinsic-size:auto_32px] [content-visibility:auto]"
      >
        <slot :item="item" />
      </li>
    </ul>
  </div>
</template>
