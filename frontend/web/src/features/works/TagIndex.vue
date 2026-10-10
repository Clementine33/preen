<script setup lang="ts">
import { computed, ref } from 'vue'
import { Modal } from '@/components/ui/modal'
import type { WorkFacet, WorksScope } from '@/lib/api/types'
import { useTagFacets } from '@/queries/works'

// The rail's tag index: a tab that opens a dialog, because a
// multi-pick with an AND/OR switch does not fit a dropdown. Source tags and the
// user's own merge by name into one entry, listed in 作品标签 and/or 我的标签
// with that vocabulary's count. Picks and the mode are staged in the dialog
// and take effect on APPLY; any other way out discards them. A pick that no
// longer matches, or whose name cannot be found, stays under SELECTED with a
// remove control, and APPLY keeps it.
//
// A history can hold a few hundred tags: only the two sections scroll, so FIND,
// the mode and APPLY never leave the dialog, and 我的标签 — the short, personal
// list — comes first instead of under the source vocabulary's long tail.

/**
 * The applied tag choice, exactly as the address spells it: one group of refs
 * (`s:`/`u:` ids) per picked tag, and how the groups combine. Names are looked
 * up for display only, so a pick whose name cannot be found is still a pick.
 */
export interface TagSelection {
  groups: string[][]
  mode: 'all' | 'any'
}

interface Entry {
  name: string
  sourceRefs: string[]
  userRefs: string[]
  sourceCount: number
  userCount: number
}

/** One staged pick: its refs, and its name when one could be found. */
interface Staged {
  name: string | null
  refs: string[]
}

/** The wall's other filters: the index offers only tags they can combine with. */
const props = defineProps<{ scope: WorksScope }>()
const model = defineModel<TagSelection>({ required: true })

// ADR 0009 keeps same-named entities apart; the index offers the name once.
function byName(facets: { source_tags: WorkFacet[]; user_tags: WorkFacet[] }): Map<string, Entry> {
  const out = new Map<string, Entry>()
  const fold = (list: WorkFacet[], kind: 's' | 'u') => {
    for (const facet of list) {
      const entry = out.get(facet.name) ??
        { name: facet.name, sourceRefs: [], userRefs: [], sourceCount: 0, userCount: 0 }
      if (kind === 's') {
        entry.sourceRefs.push(`s:${facet.id}`)
        entry.sourceCount += facet.work_count
      } else {
        entry.userRefs.push(`u:${facet.id}`)
        entry.userCount += facet.work_count
      }
      out.set(facet.name, entry)
    }
  }
  fold(facets.source_tags, 's')
  fold(facets.user_tags, 'u')
  return out
}

const { facets } = useTagFacets(() => props.scope)
const entries = computed(() => byName(facets.value))

function nameIn(map: Map<string, Entry>, refs: string[]): string | null {
  for (const entry of map.values()) {
    if (refs.some((ref) => entry.sourceRefs.includes(ref) || entry.userRefs.includes(ref))) return entry.name
  }
  return null
}

// A pick the other filters have narrowed away is missing from the scoped
// lists; its name comes from the library's whole vocabulary, asked for only
// while such a pick exists.
const unresolved = computed(() => model.value.groups.some((refs) => nameIn(entries.value, refs) === null))
const { facets: libraryFacets } = useTagFacets(
  () => (props.scope.library ? { library: props.scope.library } : {}),
  unresolved,
)
const libraryEntries = computed(() => byName(libraryFacets.value))
const nameOf = (refs: string[]) => nameIn(entries.value, refs) ?? nameIn(libraryEntries.value, refs)

/** The trigger's count: applied groups, named or not. */
const applied = computed(() => model.value.groups.length)

const open = ref(false)
const search = ref('')
const staged = ref<Staged[]>([])
const stagedMode = ref<TagSelection['mode']>('all')

function openIndex(): void {
  staged.value = model.value.groups.map((refs) => ({ name: nameOf(refs), refs }))
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

const isStaged = (name: string) => staged.value.some((pick) => pick.name === name)
/** A staged pick the current lists do not carry: kept, and marked as matching nothing here. */
const unmatched = (pick: Staged) => pick.name === null || !entries.value.has(pick.name)

function toggle(entry: Entry): void {
  staged.value = isStaged(entry.name)
    ? staged.value.filter((pick) => pick.name !== entry.name)
    : [...staged.value, { name: entry.name, refs: [...entry.sourceRefs, ...entry.userRefs] }]
}

function unstage(pick: Staged): void {
  staged.value = staged.value.filter((candidate) => candidate !== pick)
}

function apply(): void {
  // Every staged pick is committed as its refs, so a pick whose name could not
  // be found is kept unless the user removed it.
  model.value = { groups: staged.value.map((pick) => pick.refs), mode: stagedMode.value }
  open.value = false
}

const active = computed(() => applied.value > 0)
</script>

<template>
  <div class="ti" :class="{ active }">
    <button class="ti-trigger" type="button" aria-haspopup="dialog" :aria-expanded="open" @click="openIndex">
      <span class="lbl ti-lbl">TAG</span>
      <span v-if="active" class="ti-value">{{ applied }} · {{ model.mode === 'all' ? 'AND' : 'OR' }}</span>
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
          <button v-for="pick in staged" :key="pick.refs.join('|')" class="chip on" type="button"
            :title="`移除 ${pick.name ?? '未知标签'}`" @click="unstage(pick)">{{ pick.name ?? '未知标签' }}<span
              v-if="unmatched(pick)" class="lbl n" title="当前筛选下无匹配">0</span><span aria-hidden="true">×</span></button>
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
              :class="{ on: isStaged(entry.name) }" :aria-pressed="isStaged(entry.name)"
              @click="toggle(entry)">{{ entry.name }}<span class="lbl n">{{ section.count(entry) }}</span></button>
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
