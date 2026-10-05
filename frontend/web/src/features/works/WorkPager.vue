<script setup lang="ts">
import { computed } from 'vue'

// The wall's numbered pager: the first and last page, a window of two either
// side of the current one, and an ellipsis for each gap. It only reports the
// page asked for; the address is the page's to change.

const props = defineProps<{ page: number; pageCount: number }>()
const emit = defineEmits<{ go: [page: number] }>()

/** Page numbers to draw, with null standing for a gap. */
const items = computed(() => pagerItems(props.page, props.pageCount))
</script>

<script lang="ts">
export function pagerItems(page: number, count: number): (number | null)[] {
  const wanted = new Set([1, count])
  for (let n = page - 2; n <= page + 2; n++) if (n >= 1 && n <= count) wanted.add(n)
  const sorted = [...wanted].sort((a, b) => a - b)
  const out: (number | null)[] = []
  for (const n of sorted) {
    const last = out.at(-1)
    // A gap of exactly one page shows that page rather than an ellipsis.
    if (typeof last === 'number' && n - last === 2) out.push(last + 1)
    else if (typeof last === 'number' && n - last > 2) out.push(null)
    out.push(n)
  }
  return out
}
</script>

<template>
  <nav v-if="pageCount > 1" class="pager" aria-label="Pages">
    <button class="btn btn-s" type="button" aria-label="Previous page" :disabled="page <= 1"
      @click="emit('go', page - 1)">‹</button>
    <template v-for="(item, i) in items" :key="item ?? `gap-${i}`">
      <span v-if="item === null" class="lbl gap" aria-hidden="true">…</span>
      <button v-else class="btn btn-s" type="button" :class="{ on: item === page }"
        :aria-current="item === page ? 'page' : undefined" :aria-label="`Page ${item}`"
        @click="item !== page && emit('go', item)">{{ item }}</button>
    </template>
    <button class="btn btn-s" type="button" aria-label="Next page" :disabled="page >= pageCount"
      @click="emit('go', page + 1)">›</button>
  </nav>
</template>

<style scoped>
.pager {
  display: flex; align-items: center; justify-content: center; gap: 4px;
  padding: var(--sp-2) var(--sp-6); border-top: 1px solid var(--wire-fine);
  font-variant-numeric: tabular-nums;
}
.pager .btn { min-width: 28px; justify-content: center; }
.pager .btn:disabled { opacity: .4; cursor: not-allowed; }
.pager .on { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }
.gap { padding: 0 4px; }
</style>
