<script setup lang="ts">
import { ref } from 'vue'

/**
 * The frame of a Task's settings next to its results: a column that scrolls
 * on its own, with the apply bar fixed at its foot. In a narrow container the
 * body folds into a one-line summary that opens in place; the bar stays
 * visible while there are unapplied edits.
 */
defineProps<{
  /** One line that says what the settings currently are. */
  summary: string
  /** Fold the body into the summary. */
  collapsible: boolean
}>()
const open = ref(false)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="task-settings">
    <button
      v-if="collapsible"
      type="button"
      class="flex h-9 w-full cursor-pointer items-center gap-2 border-b border-[var(--wire-fine)] px-3 text-left hover:bg-muted"
      :aria-expanded="open"
      data-testid="settings-summary"
      @click="open = !open"
    >
      <span class="min-w-0 flex-1 truncate text-xs font-medium">{{ summary }}</span>
      <span aria-hidden="true" class="text-[11px]">{{ open ? '▴' : '▾' }}</span>
    </button>
    <div v-show="!collapsible || open" class="min-h-0 flex-1 space-y-4 overflow-y-auto p-3" data-testid="settings-body">
      <slot />
    </div>
    <slot name="bar" />
  </div>
</template>
