<script setup lang="ts" generic="T">
/**
 * A single-line list for the task pages, where a record can hold thousands of
 * folders. It does not scroll: the results pane around it is the page's one
 * scroller. Every row is rendered, but `content-visibility: auto` makes the
 * browser skip layout and paint of the rows out of view; `auto 32px` reserves
 * the row height until it has been drawn once, so the scrollbar stays put. The
 * rows are plain DOM, so find-in-page and assistive tech still reach all of
 * them. Keep the reserved height and the row's height class together: the
 * default is one 32px line, a caller with taller rows passes both in `rowClass`.
 */
withDefaults(
  defineProps<{
    items: T[]
    itemKey: (item: T) => string
    testid: string
    rowClass?: string
  }>(),
  { rowClass: 'h-8 [contain-intrinsic-size:auto_32px]' },
)
defineSlots<{ default(props: { item: T }): unknown }>()
</script>

<template>
  <div class="text-xs">
    <ul class="m-0 w-full list-none p-0" :data-testid="testid">
      <li
        v-for="item in items"
        :key="itemKey(item)"
        class="grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 border-b border-border [content-visibility:auto]"
        :class="rowClass"
      >
        <slot :item="item" />
      </li>
    </ul>
  </div>
</template>
