<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useMediaQuery, WALL_MOBILE_QUERY } from '@/composables/use-media-query'
import { usePopupMenu } from './use-popup-menu'

// One select of the filter rail: a rail tab that unfolds the
// library switcher's specimen panel. A tab with a value set reads inverted, the
// way a pressed tab does; long lists (voice actors, circles) carry a search box
// that narrows the options by name.
//
// A history can hold about a thousand voice actors or circles, so the list is
// virtualized: only the rows in view exist, closed or open. There is no
// keyboard navigation of its own: a long list is narrowed with the search box.
//
// A narrow screen presents the panel as a bottom sheet: the same panel as a
// native modal <dialog> (top layer, focus trap, Esc), closed by its CLOSE
// button or a tap on the backdrop. The dialog's own `close` event keeps `open`
// in step, so Esc and the Android back gesture need no handler of their own.

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
const menuEl = ref<HTMLElement | null>(null)
const narrow = useMediaQuery(WALL_MOBILE_QUERY)
const coarse = useMediaQuery('(pointer: coarse)')

// jsdom has no showModal, so tests render the same DOM by the plain open attribute.
watch(open, (value) => {
  const el = menuEl.value as HTMLDialogElement | null
  if (!narrow.value || el === null || el.tagName !== 'DIALOG' || el.open === value) return
  if (value) {
    if (typeof el.showModal === 'function') el.showModal()
    else el.setAttribute('open', '')
  } else if (typeof el.close === 'function') el.close()
  else el.removeAttribute('open')
})
/** A tap on the dialog itself, outside its content, is the backdrop. */
function onSheetClick(event: MouseEvent): void {
  if (event.target === menuEl.value) menu.close(true)
}

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

/** One row including its rule, the height `.rs-item` draws: 44px for a finger. */
const row = computed(() => (coarse.value ? 44 : 32))
const LIST_MAX = 320
const virtualizer = useVirtualizer(
  computed(() => ({
    count: shown.value.length,
    getScrollElement: () => listEl.value,
    estimateSize: () => row.value,
    overscan: 6,
    // The panel is laid out while hidden, but jsdom has no layout at all.
    initialRect: { width: 280, height: LIST_MAX },
  })),
)
const virtualRows = computed(() => virtualizer.value.getVirtualItems())
const listHeight = computed(() => `${Math.min(LIST_MAX, shown.value.length * row.value)}px`)
// A narrower list starts from its top, not from wherever the last one was.
watch(search, () => virtualizer.value.scrollToOffset(0))
// Cached row measurements are of the old height.
watch(row, () => virtualizer.value.measure())
// Opening shows the current value in place, even when the search box takes
// the focus: in a thousand rows it would otherwise sit far out of view.
watch(open, (value) => {
  const index = shown.value.findIndex((option) => option.value === model.value)
  if (value && index > 0) virtualizer.value.scrollToIndex(index, { align: 'center' })
})

const menu = usePopupMenu({ root, trigger, open })

const current = computed(() => props.options.find((option) => option.value === model.value) ?? null)
const active = computed(() => props.marksActive === true && model.value !== '')

function choose(value: string): void {
  model.value = value
  menu.close(true)
}

const chevron = 'M0 2.5h10L5 8z'
</script>

<template>
  <div ref="root" class="rs" :class="{ open, active }" @focusout="menu.onFocusOut">
    <button ref="trigger" class="rs-trigger" type="button" aria-haspopup="listbox" :aria-expanded="open"
      @click="menu.setOpen(!open)">
      <span class="lbl rs-lbl">{{ label }}</span>
      <span v-if="current && (current.value !== '' || !marksActive)" class="rs-value">{{ current.label }}</span>
      <svg class="rs-chev" viewBox="0 0 10 10" aria-hidden="true"><path :d="chevron" /></svg>
    </button>
    <component :is="narrow ? 'dialog' : 'div'" ref="menuEl" class="rs-menu" data-own-keys :aria-label="narrow ? label : undefined"
      @click="onSheetClick" @close="open = false">
      <div class="strip">
        <span class="strip-name lbl">{{ label }}</span>
        <span class="lbl">{{ options.length - (options[0]?.value === '' ? 1 : 0) }}</span>
        <button v-if="narrow" class="lbl rs-close" type="button" @click="menu.close(true)">CLOSE</button>
      </div>
      <label v-if="searchable" class="rs-search">
        <span class="lbl">FIND</span>
        <input v-model="search" type="search" :aria-label="`Find ${label.toLowerCase()}`" />
      </label>
      <div ref="listEl" class="rs-list" role="listbox" :aria-label="label" :style="narrow ? undefined : { height: listHeight }">
        <div class="rs-rows" :style="{ height: `${virtualizer.getTotalSize()}px` }">
          <button v-for="row in virtualRows" :key="shown[row.index].value" class="rs-item" type="button"
            role="option" :data-index="row.index" :aria-selected="shown[row.index].value === model"
            :aria-setsize="shown.length" :aria-posinset="row.index + 1"
            :style="{ top: `${row.start}px`, '--d': `${Math.min(row.index, 5) * 24 + 30}ms` }"
            @click="choose(shown[row.index].value)">
            <span class="rs-mark"></span>
            <span class="rs-name">{{ shown[row.index].label }}</span>
            <span v-if="shown[row.index].count !== undefined" class="lbl rs-count">{{ shown[row.index].count }}</span>
          </button>
        </div>
      </div>
      <p v-if="shown.length === 0" class="rs-empty lbl">NO MATCH</p>
    </component>
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
  clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-out);
}
.rs-trigger:focus-visible::before, .rs.open .rs-trigger::before {
  clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
}
@media (hover: hover) and (pointer: fine) {
  .rs-trigger:hover::before {
    clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
  }
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
  transition: clip-path 100ms var(--ease-out), visibility 0s linear 100ms;
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
  clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-out);
}
.rs-item:focus-visible::before {
  clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
}
@media (hover: hover) and (pointer: fine) {
  .rs-item:hover::before {
    clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
  }
}
.rs-item:focus-visible { outline: none; }
.rs-mark { width: 6px; height: 6px; justify-self: center; }
.rs-item:focus-visible .rs-mark { background: var(--wire-mid); }
@media (hover: hover) and (pointer: fine) {
  .rs-item:hover .rs-mark { background: var(--wire-mid); }
}
.rs-name { font-size: var(--fs-s); font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.rs-item[aria-selected='true'] { color: var(--invert-fg); }
.rs-item[aria-selected='true']::before { background: var(--invert-bg); clip-path: inset(0 0 0 0); }
.rs-item[aria-selected='true'] .rs-mark { background: var(--invert-fg); }
.rs-item[aria-selected='true'] .lbl { color: var(--invert-fg); }
.rs-empty { padding: var(--sp-3) var(--sp-2); }

/* A finger needs 44px: the trigger and the rows (the virtualizer's `row`). */
@media (pointer: coarse) {
  .rs-trigger { height: auto; min-height: 44px; }
  .rs-item { height: 44px; }
}

@media (max-width: 768px) {
  /* Tabs are compact on a phone: the label alone, the value lives in the
     panel. The panel is a bottom sheet: a modal <dialog> anchored to the
     bottom edge, the list scrolling under a fixed header and search box. */
  .rs { position: static; }
  .rs-value { display: none; }
  dialog.rs-menu .strip { height: auto; min-height: 20px; justify-content: flex-start; }
  dialog.rs-menu .strip-name { margin-right: auto; }
  .rs-close { margin-left: var(--sp-3); min-height: 44px; padding: 0 var(--sp-2); color: var(--fg-base); }
  dialog.rs-menu {
    position: fixed; inset: auto 0 0 0; z-index: auto; margin: 0; padding: 0;
    width: 100%; max-width: 100%; max-height: 70dvh; color: inherit;
    border-width: 1px 0 0; box-shadow: none;
  }
  dialog.rs-menu[open] {
    display: flex; flex-direction: column;
    animation: rs-sheet-in 220ms var(--ease-out);
  }
  dialog.rs-menu::backdrop { background: rgb(0 0 0 / .5); }
  dialog.rs-menu .rs-list { flex: 1 1 auto; min-height: 0; }
  @keyframes rs-sheet-in { from { transform: translateY(100%); } }
}
@media (max-width: 768px) and (prefers-reduced-motion: reduce) {
  dialog.rs-menu[open] { animation: none; }
}
</style>
