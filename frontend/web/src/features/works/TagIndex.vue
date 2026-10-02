<script setup lang="ts">
import { computed, ref } from 'vue'
import { Modal } from '@/components/ui/modal'
import type { WorkFacet } from '@/lib/api/types'
import { useTagFacets } from '@/queries/works'

// The rail's tag index (ADR 0012 §4): a tab that opens a dialog, because a
// multi-pick with an AND/OR switch does not fit a dropdown. Source tags and the
// user's own merge by name into one entry, listed in 作品标签 and/or 我的标签
// with that vocabulary's count. Picks and the mode are staged in the dialog
// and take effect on APPLY; any other way out discards them.
//
// A history can hold a few hundred tags: only the two sections scroll, so FIND,
// the mode and APPLY never leave the dialog, and 我的标签 — the short, personal
// list — comes first instead of under the source vocabulary's long tail.

/** One picked tag: its name and every id that spells it (`s:`/`u:` refs). */
export interface TagPick {
  name: string
  refs: string[]
}

export interface TagSelection {
  picks: TagPick[]
  mode: 'all' | 'any'
}

interface Entry {
  name: string
  sourceRefs: string[]
  userRefs: string[]
  sourceCount: number
  userCount: number
}

const props = defineProps<{ library: string | null }>()
const model = defineModel<TagSelection>({ required: true })

const { facets } = useTagFacets(() => props.library)

// ADR 0011 §6 keeps same-named entities apart; the index offers the name once.
const entries = computed(() => {
  const byName = new Map<string, Entry>()
  const fold = (list: WorkFacet[], kind: 's' | 'u') => {
    for (const facet of list) {
      const entry = byName.get(facet.name) ??
        { name: facet.name, sourceRefs: [], userRefs: [], sourceCount: 0, userCount: 0 }
      if (kind === 's') {
        entry.sourceRefs.push(`s:${facet.id}`)
        entry.sourceCount += facet.work_count
      } else {
        entry.userRefs.push(`u:${facet.id}`)
        entry.userCount += facet.work_count
      }
      byName.set(facet.name, entry)
    }
  }
  fold(facets.value.source_tags, 's')
  fold(facets.value.user_tags, 'u')
  return byName
})

const open = ref(false)
const search = ref('')
const staged = ref<string[]>([])
const stagedMode = ref<TagSelection['mode']>('all')

function openIndex(): void {
  staged.value = model.value.picks.map((pick) => pick.name)
  stagedMode.value = model.value.mode
  search.value = ''
  open.value = true
}

/** Each section by its own vocabulary's count, most first; ties by name. */
const sections = computed(() => {
  const needle = search.value.trim().toLowerCase()
  const all = [...entries.value.values()].filter((entry) => entry.name.toLowerCase().includes(needle))
  const ordered = (count: (entry: Entry) => number) =>
    all.filter((entry) => count(entry) > 0).sort((a, b) => count(b) - count(a) || a.name.localeCompare(b.name))
  return [
    { title: '我的标签', entries: ordered((entry) => entry.userCount), count: (entry: Entry) => entry.userCount },
    { title: '作品标签', entries: ordered((entry) => entry.sourceCount), count: (entry: Entry) => entry.sourceCount },
  ]
})

function toggle(name: string): void {
  staged.value = staged.value.includes(name)
    ? staged.value.filter((picked) => picked !== name)
    : [...staged.value, name]
}

function apply(): void {
  // A staged name the current scope no longer lists cannot be asked for.
  const picks = staged.value.flatMap((name) => {
    const entry = entries.value.get(name)
    return entry ? [{ name, refs: [...entry.sourceRefs, ...entry.userRefs] }] : []
  })
  model.value = { picks, mode: stagedMode.value }
  open.value = false
}

const active = computed(() => model.value.picks.length > 0)
</script>

<template>
  <div class="ti" :class="{ active }">
    <button class="ti-trigger" type="button" aria-haspopup="dialog" :aria-expanded="open" @click="openIndex">
      <span class="lbl ti-lbl">TAG</span>
      <span v-if="active" class="ti-value">{{ model.picks.length }} · {{ model.mode === 'all' ? 'AND' : 'OR' }}</span>
    </button>
    <Modal v-model:open="open" title="TAG INDEX" class="flex w-[min(44rem,calc(100vw-2rem))] flex-col overflow-hidden">
      <div class="ti-body">
        <div class="ti-bar">
          <label class="ti-find">
            <span class="lbl">FIND</span>
            <input v-model="search" type="search" aria-label="Find tag" />
          </label>
          <div class="ti-mode" role="radiogroup" aria-label="Match mode">
            <button v-for="mode in (['all', 'any'] as const)" :key="mode" type="button" role="radio"
              :aria-checked="stagedMode === mode" @click="stagedMode = mode">{{ mode === 'all' ? 'AND' : 'OR' }}</button>
          </div>
        </div>

        <div class="ti-picked" aria-label="Selected tags">
          <span class="lbl">SELECTED</span>
          <button v-for="name in staged" :key="name" class="chip on" type="button" :title="`移除 ${name}`"
            @click="toggle(name)">{{ name }}<span aria-hidden="true">×</span></button>
          <span v-if="staged.length === 0" class="lbl none">NONE</span>
        </div>

        <div class="ti-scroll">
        <section v-for="section in sections" :key="section.title" class="ti-section">
          <div class="strip">
            <span class="strip-name lbl">{{ section.title }}</span>
            <span class="lbl">{{ section.entries.length }}</span>
          </div>
          <div class="ti-chips">
            <button v-for="entry in section.entries" :key="entry.name" class="chip" type="button"
              :class="{ on: staged.includes(entry.name) }" :aria-pressed="staged.includes(entry.name)"
              @click="toggle(entry.name)">{{ entry.name }}<span class="lbl n">{{ section.count(entry) }}</span></button>
            <p v-if="section.entries.length === 0" class="lbl none">{{ search ? 'NO MATCH' : 'NO TAGS' }}</p>
          </div>
        </section>
        </div>

        <div class="ti-foot">
          <button class="btn" type="button" :disabled="staged.length === 0" @click="staged = []">CLEAR</button>
          <button class="btn btn-primary" type="button" @click="apply">APPLY</button>
        </div>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
/* The trigger is a rail tab, the same block RailSelect draws. */
.ti-trigger {
  position: relative; isolation: isolate;
  height: 24px; padding: 0 var(--sp-3); white-space: nowrap;
  display: inline-flex; align-items: center; gap: var(--sp-2); color: var(--fg-muted);
}
.ti-trigger::before {
  content: ''; position: absolute; inset: 0; z-index: -1; background: var(--bg-subtle);
  clip-path: inset(0 100% 0 0); transition: clip-path var(--dur-out) var(--ease-in);
}
.ti-trigger:hover::before, .ti-trigger:focus-visible::before {
  clip-path: inset(0 0 0 0); transition: clip-path var(--dur-in) var(--ease-out);
}
.ti-trigger:hover { color: var(--fg-base); }
.ti-trigger:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 2px; }
.ti-value { font-size: var(--fs-s); font-weight: 700; letter-spacing: 0; }
.ti.active .ti-trigger { background: var(--invert-bg); color: var(--invert-fg); }
.ti.active .ti-trigger::before { display: none; }
.ti.active .ti-lbl, .ti.active .ti-value { color: var(--invert-fg); }

/* The dialog is a flex column capped at the viewport: the body takes what the
   title row leaves, and inside it only .ti-scroll gives way. */
.ti-body { flex: 1 1 auto; min-height: 0; display: flex; flex-direction: column; gap: var(--sp-3); min-width: 0; }
.ti-scroll { flex: 1 1 auto; min-height: 0; overflow-y: auto; display: flex; flex-direction: column; gap: var(--sp-3); }
.ti-bar { display: flex; align-items: center; gap: var(--sp-3); }
.ti-find { flex: 1; display: flex; align-items: center; gap: var(--sp-2); min-width: 0; }
.ti-find input {
  flex: 1; min-width: 0; height: 28px; padding: 0 var(--sp-2);
  border: 1px solid var(--wire-control); background: transparent; color: var(--fg-base); font-size: var(--fs-s);
}
.ti-find input:focus { border-color: var(--wire-strong); outline: none; }

/* AND / OR: a two-cell switch; the set mode is inverted, like a pressed tab. */
.ti-mode { display: inline-flex; border: 1px solid var(--wire-control); }
.ti-mode button {
  height: 26px; padding: 0 var(--sp-3);
  font-family: var(--font-mono); font-size: var(--fs-xs); letter-spacing: .12em; color: var(--fg-muted);
}
.ti-mode button + button { border-left: 1px solid var(--wire-control); }
.ti-mode button[aria-checked='true'] { background: var(--invert-bg); color: var(--invert-fg); font-weight: 700; }
.ti-mode button:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 2px; }

.ti-picked {
  display: flex; flex-wrap: wrap; align-items: center; gap: 4px;
  min-height: 28px; padding: var(--sp-1) 0; border-bottom: 1px solid var(--wire-fine);
  /* Three rows of picks at most; a long selection scrolls in place. */
  flex-shrink: 0; max-height: 84px; overflow-y: auto;
}
.ti-picked > .lbl:first-child { margin-right: var(--sp-2); }

/* Each strip stays pinned while its section scrolls under it. */
.ti-section .strip { position: sticky; top: 0; z-index: 1; margin-bottom: var(--sp-2); }
.ti-chips { display: flex; flex-wrap: wrap; gap: 4px; }
.chip {
  height: 22px; padding: 0 6px; display: inline-flex; align-items: center; gap: 6px;
  border: 1px solid var(--wire-mid); font-size: var(--fs-xs); color: var(--fg-base);
}
.chip:hover { border-color: var(--wire-strong); }
.chip:focus-visible { outline: 1px solid var(--wire-strong); outline-offset: 2px; }
.chip .n { letter-spacing: .04em; }
.chip.on { background: var(--invert-bg); color: var(--invert-fg); border-color: var(--invert-bg); font-weight: 700; }
.chip.on .n { color: var(--invert-fg); }
.none { letter-spacing: .04em; text-transform: none; }

.ti-foot {
  display: flex; justify-content: flex-end; gap: var(--sp-2);
  padding-top: var(--sp-3); border-top: 1px solid var(--wire-fine);
}
.ti-foot .btn:disabled { opacity: .5; cursor: not-allowed; }
</style>
