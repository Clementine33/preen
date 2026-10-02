<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { usePopupMenu } from './use-popup-menu'

// One select of the filter rail (ADR 0012 §4): a rail tab that unfolds the
// library switcher's specimen panel. A tab with a value set reads inverted, the
// way a pressed tab does; long lists (voice actors, circles) carry a search box
// that narrows the options by name.
//
// A history can hold about a thousand voice actors or circles, so the list is
// virtualized: only the rows in view exist, closed or open, and the keyboard
// moves by index rather than through the DOM.

export interface RailOption {
  /** '' is the "no constraint" entry every select opens with. */
  value: string
  label: string
  /** Listed works carrying the value, shown right-aligned. */
  count?: number
}

const props = defineProps<{
  label: string
  options: RailOption[]
  /** Whether a set value inverts the tab: a filter does, the order does not. */
  marksActive?: boolean
  searchable?: boolean
}>()

const model = defineModel<string>({ required: true })
const open = ref(false)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const listEl = ref<HTMLElement | null>(null)

const search = ref('')
watch(open, (value) => {
  if (!value) search.value = ''
})
const shown = computed(() => {
  const needle = search.value.trim().toLowerCase()
  if (needle === '') return props.options
  // The empty entry stays reachable while narrowing: clearing is one step.
  return props.options.filter((option) => option.value === '' || option.label.toLowerCase().includes(needle))
})

/** One row: 32px including its rule, the height `.rs-item` draws. */
const ROW = 32
const LIST_MAX = 320
const virtualizer = useVirtualizer(
  computed(() => ({
    count: shown.value.length,
    getScrollElement: () => listEl.value,
    estimateSize: () => ROW,
    overscan: 6,
    // The panel is laid out while hidden, but jsdom has no layout at all.
    initialRect: { width: 280, height: LIST_MAX },
  })),
)
const virtualRows = computed(() => virtualizer.value.getVirtualItems())
const listHeight = computed(() => `${Math.min(LIST_MAX, shown.value.length * ROW)}px`)
// A narrower list starts from its top, not from wherever the last one was.
watch(search, () => virtualizer.value.scrollToOffset(0))
// Opening shows the current value in place, even when the search box takes
// the focus: in a thousand rows it would otherwise sit far out of view.
watch(open, (value) => {
  const index = shown.value.findIndex((option) => option.value === model.value)
  if (value && index > 0) virtualizer.value.scrollToIndex(index, { align: 'center' })
})

/**
 * Scroll one option into view, then focus it once its row exists: the
 * virtualizer settles a jump over the next frames, so the row may not be drawn
 * on the tick after the scroll.
 */
function focusRow(index: number): void {
  if (index < 0 || index >= shown.value.length) return
  virtualizer.value.scrollToIndex(index)
  let frames = 0
  const attempt = () => {
    const row = listEl.value?.querySelector<HTMLElement>(`[data-index="${index}"]`)
    if (row) row.focus({ preventScroll: true })
    else if (open.value && frames++ < 5) requestAnimationFrame(attempt)
  }
  void nextTick(attempt)
}

// The row the keyboard is on. It moves the moment a key arrives, not when the
// focus lands a frame later: a held arrow key would otherwise re-read a row it
// already left. A row focused any other way (pointer, Tab) resets it, and so
// does the search box (-1: the next step lands on row 0).
let cursor = -1
function onListFocus(event: FocusEvent): void {
  const index = (event.target as HTMLElement).dataset.index
  cursor = index === undefined ? -1 : Number(index)
}

const menu = usePopupMenu({
  root,
  trigger,
  open,
  step: (delta) => {
    cursor = Math.min(shown.value.length - 1, Math.max(0, cursor + delta))
    focusRow(cursor)
  },
  focusCurrent: () => {
    cursor = Math.max(0, shown.value.findIndex((option) => option.value === model.value))
    focusRow(cursor)
  },
})

const current = computed(() => props.options.find((option) => option.value === model.value) ?? null)
const active = computed(() => props.marksActive === true && model.value !== '')

function choose(value: string): void {
  model.value = value
  menu.close(true)
}

const chevron = 'M0 2.5h10L5 8z'
</script>

<template>
  <div ref="root" class="rs" :class="{ open, active }" @keydown="menu.onKeydown" @focusout="menu.onFocusOut"
    @focusin="onListFocus">
    <button ref="trigger" class="rs-trigger" type="button" aria-haspopup="listbox" :aria-expanded="open"
      @click="menu.setOpen(!open)">
      <span class="lbl rs-lbl">{{ label }}</span>
      <span v-if="current && (current.value !== '' || !marksActive)" class="rs-value">{{ current.label }}</span>
      <svg class="rs-chev" viewBox="0 0 10 10" aria-hidden="true"><path :d="chevron" /></svg>
    </button>
    <div class="rs-menu" data-own-keys>
      <div class="strip">
        <span class="strip-name lbl">{{ label }}</span>
        <span class="lbl">{{ options.length - (options[0]?.value === '' ? 1 : 0) }}</span>
      </div>
      <label v-if="searchable" class="rs-search">
        <span class="lbl">FIND</span>
        <input v-model="search" type="search" :aria-label="`Find ${label.toLowerCase()}`" />
      </label>
      <div ref="listEl" class="rs-list" role="listbox" :aria-label="label" :style="{ height: listHeight }">
        <div class="rs-rows" :style="{ height: `${virtualizer.getTotalSize()}px` }">
          <button v-for="row in virtualRows" :key="shown[row.index].value" class="rs-item" type="button"
            role="option" :data-index="row.index" :aria-selected="shown[row.index].value === model"
            :aria-setsize="shown.length" :aria-posinset="row.index + 1"
            :style="{ top: `${row.start}px`, '--d': `${Math.min(row.index, 8) * 24 + 30}ms` }"
            @click="choose(shown[row.index].value)">
            <span class="rs-mark"></span>
            <span class="rs-name">{{ shown[row.index].label }}</span>
            <span v-if="shown[row.index].count !== undefined" class="lbl rs-count">{{ shown[row.index].count }}</span>
          </button>
        </div>
      </div>
      <p v-if="shown.length === 0" class="rs-empty lbl">NO MATCH</p>
    </div>
  </div>
</template>

<style scoped>
.rs { position: relative; }

/* The trigger is a rail tab: the same 24px block, the same left-to-right wipe. */
.rs-trigger {
  position: relative; isolation: isolate;
  height: 24px; padding: 0 var(--sp-2) 0 var(--sp-3); white-space: nowrap;
  display: inline-flex; align-items: center; gap: var(--sp-2); color: var(--fg-muted);
}
.rs-trigger::before {
  content: ''; position: absolute; inset: 0; z-index: -1; background: var(--bg-subtle);
  clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-in);
}
.rs-trigger:hover::before, .rs-trigger:focus-visible::before, .rs.open .rs-trigger::before {
  clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
}
.rs-trigger:hover, .rs.open .rs-trigger { color: var(--fg-base); }
.rs-trigger:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 2px; }
.rs-value {
  font-size: var(--fs-s); font-weight: 700; color: var(--fg-base); letter-spacing: 0;
  max-width: 160px; overflow: hidden; text-overflow: ellipsis;
}
.rs-chev { width: 8px; height: 8px; fill: currentColor; transition: transform var(--dur-in) var(--ease-out); }
.rs.open .rs-chev { transform: rotate(180deg); }

/* A set filter reads as a pressed tab: the bone block, inverted text. */
.rs.active .rs-trigger { background: var(--invert-bg); color: var(--invert-fg); }
.rs.active .rs-trigger::before { display: none; }
.rs.active .rs-lbl, .rs.active .rs-value { color: var(--invert-fg); }

/* The panel: the library switcher's specimen on a 4px ghost frame. */
.rs-menu {
  position: absolute; left: 0; top: calc(100% + 8px); z-index: 60;
  width: 280px;
  background: var(--grain), var(--bg-panel);
  border: 1px solid var(--wire-strong);
  box-shadow: 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid);
  visibility: hidden; clip-path: inset(0 0 100% 0);
  transition: clip-path 100ms var(--ease-in), visibility 0s linear 100ms;
}
.rs.open .rs-menu {
  visibility: visible; clip-path: inset(-8px -8px -8px -8px);
  transition: clip-path 200ms var(--ease-out), visibility 0s;
}
.rs-menu .strip { margin: var(--sp-2) var(--sp-2) var(--sp-1); }
.rs-search { display: flex; align-items: center; gap: var(--sp-2); margin: var(--sp-2) var(--sp-2) var(--sp-1); }
.rs-search input {
  flex: 1; min-width: 0; height: 24px; padding: 0 var(--sp-2);
  border: 1px solid var(--wire-control); background: transparent; color: var(--fg-base);
  font-size: var(--fs-s);
}
.rs-search input:focus { border-color: var(--wire-strong); outline: none; }

.rs-list { overflow-y: auto; margin: 0 var(--sp-2) var(--sp-2); }
/* Rows sit at their virtual offset; `top`, not a transform, because the
   entrance animation already owns the row's transform. */
.rs-rows { position: relative; }
.rs-item {
  position: absolute; left: 0; isolation: isolate; width: 100%;
  display: grid; grid-template-columns: 14px 1fr auto; align-items: center; gap: var(--sp-2);
  height: 32px; padding: 0 var(--sp-2) 0 var(--sp-1); text-align: left;
  color: var(--fg-base); border-bottom: 1px solid var(--wire-fine);
  opacity: 0; transform: translateY(-4px);
  transition: opacity 80ms linear, transform 80ms linear;
}
.rs.open .rs-item {
  opacity: 1; transform: none;
  transition: opacity 140ms linear var(--d, 30ms), transform 180ms var(--ease-out) var(--d, 30ms);
}
.rs-item::before {
  content: ''; position: absolute; inset: 0; z-index: -1; background: var(--bg-subtle);
  clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-in);
}
.rs-item:hover::before, .rs-item:focus-visible::before {
  clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
}
.rs-item:focus-visible { outline: none; }
.rs-mark { width: 6px; height: 6px; justify-self: center; }
.rs-item:hover .rs-mark, .rs-item:focus-visible .rs-mark { background: var(--wire-mid); }
.rs-name { font-size: var(--fs-s); font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.rs-item[aria-selected='true'] { color: var(--invert-fg); }
.rs-item[aria-selected='true']::before { background: var(--invert-bg); clip-path: inset(0 0 0 0); }
.rs-item[aria-selected='true'] .rs-mark { background: var(--invert-fg); }
.rs-item[aria-selected='true'] .lbl { color: var(--invert-fg); }
.rs-empty { padding: var(--sp-3) var(--sp-2); }

@media (max-width: 768px) {
  /* Tabs are compact on a phone: the label alone, the value lives in the
     panel. The panel hugs the rail's gutters instead of its trigger. */
  .rs { position: static; }
  .rs-value { display: none; }
  .rs-menu { left: var(--sp-4); right: var(--sp-4); width: auto; top: calc(100% - 4px); }
}
</style>
