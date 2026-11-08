<script setup lang="ts">
import { computed, ref } from 'vue'
import { useLibraryList } from '@/queries/libraries'
import { usePopupMenu } from './use-popup-menu'

// The media-library switcher: the trigger unfolds a specimen
// panel that cascades in row by row. It is a pure picker over the wall's
// address: the page passes the address's library and writes the pick back; the
// switcher holds no library state of its own. No OFFLINE tag and no
// per-library counts, because the library DTO carries neither.

const props = defineProps<{
  /** The wall's library: `all` or a library id; null while its address names none. */
  library: string | null
}>()
const emit = defineEmits<{ select: [library: string] }>()
const open = defineModel<boolean>('open', { default: false })

const { librariesData } = useLibraryList()
const shown = computed(() => props.library === 'all'
  ? 'ALL'
  : (librariesData.value.find(library => library.id === props.library)?.name ?? '—'))

const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const menu = usePopupMenu({ root, trigger, open })

function selectLibrary(id: string): void {
  emit('select', id)
  menu.close(true)
}

const chevron = 'M0 2.5h10L5 8z'
</script>

<template>
  <div ref="root" class="lib" :class="{ open }" @keydown="menu.onKeydown" @focusout="menu.onFocusOut">
    <button
      ref="trigger" class="lib-trigger" type="button" aria-haspopup="listbox"
      :aria-expanded="open" @click="menu.setOpen(!open)"
    >
      <span class="lbl lib-lbl">LIBRARY</span>
      <span class="lib-name">{{ shown }}</span>
      <span class="lib-chev"><svg viewBox="0 0 10 10" aria-hidden="true"><path :d="chevron" /></svg></span>
    </button>
    <div class="lib-menu" data-own-keys>
      <div class="strip">
        <span class="strip-name lbl">SWITCH LIBRARY</span>
        <span class="lbl">{{ librariesData.length }} ROOTS</span>
      </div>
      <div class="lib-list" role="listbox" aria-label="Media libraries">
        <button
          class="lib-item" type="button" role="option" :aria-selected="props.library === 'all'"
          :style="{ '--d': '10ms' }" @click="selectLibrary('all')"
        >
          <span class="lib-mark" />
          <span class="lib-text">
            <span class="nm">ALL LIBRARIES</span>
            <span class="lbl path">{{ librariesData.length }} ROOTS</span>
          </span>
        </button>
        <button
          v-for="(item, i) in librariesData" :key="item.id" class="lib-item" type="button" role="option"
          :aria-selected="item.id === props.library" :style="{ '--d': `${40 + Math.min(i, 5) * 30}ms` }"
          @click="selectLibrary(item.id)"
        >
          <span class="lib-mark" />
          <span class="lib-text">
            <span class="nm">{{ item.name }}</span>
            <span class="lbl path">{{ item.root_path }}</span>
          </span>
        </button>
      </div>
      <div class="lib-foot">
        <RouterLink
          class="btn btn-s" :to="{ name: 'worksets' }"
          @click="menu.setOpen(false)"
        >MANAGE</RouterLink>
      </div>
    </div>
  </div>
</template>

<style scoped>
.lib { position: relative; }

.lib-trigger {
  position: relative; isolation: isolate;
  height: 28px; padding: 0 var(--sp-2) 0 var(--sp-3);
  display: inline-flex; align-items: center; gap: var(--sp-3);
  border: 1px solid var(--wire-fine);
  color: var(--fg-base);
  transition: border-color var(--dur-out) linear;
}
.lib-trigger::before {
  content: ''; position: absolute; inset: 0; z-index: -1; background: var(--bg-subtle);
  clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-out);
}
.lib-trigger:hover, .lib-trigger:focus-visible {
  border-color: var(--wire-mid); outline: none;
}
.lib-trigger:focus-visible::before, .lib.open .lib-trigger::before {
  clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
}
@media (hover: hover) and (pointer: fine) {
  .lib-trigger:hover::before {
    clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
  }
}
.lib.open .lib-trigger { border-color: var(--wire-strong); }
.lib-name {
  font-size: var(--fs-s); font-weight: 700; max-width: 200px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.lib-chev {
  width: 16px; height: 16px; display: grid; place-items: center;
  border-left: 1px solid var(--wire-fine); padding-left: var(--sp-2); box-sizing: content-box;
}
.lib-chev svg { width: 8px; height: 8px; fill: currentColor; transition: transform var(--dur-in) var(--ease-out); }
.lib.open .lib-chev svg { transform: rotate(180deg); }

/* The panel: a specimen on a 4px offset ghost frame, with a 1px tick above
   connecting it to the trigger. Rows cascade in ~30ms apart. */
.lib-menu {
  position: absolute; left: 0; top: calc(100% + 10px); z-index: 60;
  width: min(380px, calc(100vw - 32px));
  background: var(--grain), var(--bg-panel);
  border: 1px solid var(--wire-strong);
  box-shadow: 4px 4px 0 -1px var(--bg-canvas), 4px 4px 0 0 var(--wire-mid);
  visibility: hidden; clip-path: inset(0 0 100% 0);
  transition: clip-path 100ms var(--ease-out), visibility 0s linear 100ms;
}
.lib-menu::before {
  content: ''; position: absolute; left: 16px; top: -11px; width: 1px; height: 10px;
  background: var(--wire-strong);
}
.lib.open .lib-menu {
  visibility: visible; clip-path: inset(-12px -8px -8px -8px);
  transition: clip-path 200ms var(--ease-out), visibility 0s;
}
.lib-menu .strip { margin: var(--sp-2) var(--sp-2) var(--sp-1); }

.lib-list { padding: 0 var(--sp-2); }
.lib-item {
  position: relative; isolation: isolate; width: 100%;
  display: grid; grid-template-columns: 14px 1fr auto; align-items: center; gap: var(--sp-2);
  padding: var(--sp-2) var(--sp-2) var(--sp-2) var(--sp-1); text-align: left;
  color: var(--fg-base);
  border-bottom: 1px solid var(--wire-fine);
  opacity: 0; transform: translateY(-4px);
  transition: opacity 80ms linear, transform 80ms linear;
}
.lib.open .lib-item {
  opacity: 1; transform: none;
  transition: opacity 140ms linear var(--d, 40ms), transform 180ms var(--ease-out) var(--d, 40ms);
}
.lib-item::before {
  content: ''; position: absolute; inset: 0; z-index: -1; background: var(--bg-subtle);
  clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-out);
}
.lib-item:focus-visible::before {
  clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
}
@media (hover: hover) and (pointer: fine) {
  .lib-item:hover::before {
    clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
  }
}
.lib-item:focus-visible { outline: none; }
.lib-mark { width: 6px; height: 6px; justify-self: center; background: transparent; }
.lib-item:focus-visible .lib-mark { background: var(--wire-mid); }
@media (hover: hover) and (pointer: fine) {
  .lib-item:hover .lib-mark { background: var(--wire-mid); }
}
.lib-text { min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.lib-text .nm { font-size: var(--fs-s); font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.lib-text .lbl {
  letter-spacing: .04em; text-transform: none; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.lib-item[aria-selected='true'] { color: var(--invert-fg); }
.lib-item[aria-selected='true']::before { background: var(--invert-bg); clip-path: inset(0 0 0 0); }
.lib-item[aria-selected='true'] .lib-mark { background: var(--invert-fg); }
.lib-item[aria-selected='true'] .lbl { color: var(--invert-fg); }
.lib-foot {
  display: flex; justify-content: flex-end; align-items: center; gap: var(--sp-2);
  padding: var(--sp-3) var(--sp-3) var(--sp-3) var(--sp-3);
}
.lib-foot .btn { text-decoration: none; }

@media (max-width: 768px) {
  .lib-lbl { display: none; }
  .lib-name { max-width: 120px; }
  /* The panel hugs the page gutters instead of overflowing the trigger. */
  .lib { position: static; }
  .lib-menu { left: var(--sp-4); right: var(--sp-4); width: auto; top: 46px; }
  .lib-menu::before { display: none; }
}
</style>
