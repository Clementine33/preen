<script setup lang="ts">
/**
 * Filter tabs with counts for a results list. The selected tab is inverted,
 * like every other pressed filter in the workbench (DESIGN.md).
 */
defineProps<{ tabs: { key: string; label: string; count: number }[]; modelValue: string }>()
defineEmits<{ 'update:modelValue': [key: string] }>()
</script>

<template>
  <div role="tablist" class="flex flex-wrap gap-1" data-testid="result-tabs">
    <button
      v-for="tab in tabs"
      :key="tab.key"
      type="button"
      role="tab"
      :aria-selected="tab.key === modelValue"
      :data-testid="`tab-${tab.key}`"
      class="h-6 border border-[var(--wire-control)] px-2 text-xs hover:bg-[var(--bg-subtle)]"
      :class="tab.key === modelValue ? 'border-[var(--invert-bg)] bg-[var(--invert-bg)] text-[var(--invert-fg)] hover:bg-[var(--invert-bg)]' : ''"
      @click="$emit('update:modelValue', tab.key)"
    >
      {{ tab.label }} <span class="font-mono tabular-nums">{{ tab.count }}</span>
    </button>
  </div>
</template>
