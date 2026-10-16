<script setup lang="ts">
/**
 * One titled group of settings: a quiet heading over a 1px rule, the way the
 * workbench's detail panes title their groups. The aside slot carries a
 * group-level action beside the heading.
 */
defineProps<{ title: string }>()
</script>

<template>
  <section class="min-w-0">
    <div class="flex items-center gap-2 border-b border-[var(--wire-mid)] pb-1">
      <h3 class="flex-1 text-[11px] font-semibold text-[var(--text-secondary)]">{{ title }}</h3>
      <slot name="aside" />
    </div>
    <slot />
  </section>
</template>
