<script setup lang="ts">
import { computed, ref, watch } from 'vue'

// The personal rating range as ten cells: the first press sets one end, the
// second the other (in either order), and the range is chosen. The same cell
// twice is an exact score. Each opening of the panel starts a fresh pick.

const props = defineProps<{
  /** The chosen range, or null when the rating is not a range. */
  range: { from: number, to: number } | null
  open: boolean
}>()
const emit = defineEmits<{ pick: [from: number, to: number] }>()

const SCORES = Array.from({ length: 10 }, (_, i) => i + 1)
const start = ref<number | null>(null)
watch(() => props.open, () => {
  start.value = null
})

const lit = computed(() => start.value === null
  ? props.range
  : { from: start.value, to: start.value })

function press(score: number): void {
  if (start.value === null) {
    start.value = score
    return
  }
  const from = Math.min(start.value, score)
  const to = Math.max(start.value, score)
  start.value = null
  emit('pick', from, to)
}
</script>

<template>
  <div class="rb" role="group" aria-label="Rating range">
    <button
      v-for="score in SCORES" :key="score" class="rb-cell" type="button"
      :aria-pressed="lit !== null && score >= lit.from && score <= lit.to"
      @click="press(score)"
    >
      {{ score }}
    </button>
  </div>
</template>

<style scoped>
.rb { display: grid; grid-template-columns: repeat(10, 1fr); gap: 2px; margin: var(--sp-2); }
.rb-cell {
  height: 32px; font-size: var(--fs-s); font-weight: 700; color: var(--fg-base);
  border: 1px solid var(--wire-control);
}
.rb-cell:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 1px; }
@media (hover: hover) and (pointer: fine) {
  .rb-cell:hover { background: var(--bg-subtle); }
}
.rb-cell[aria-pressed='true'] { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); }
@media (pointer: coarse) {
  .rb-cell { height: 44px; }
}
</style>
