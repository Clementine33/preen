<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { Button } from '@/components/ui/button'
import type { RenameDraftMember, WorksetMember } from '@/lib/api/types'

/**
 * Names one folder by hand. A batch is often the whole library, so nothing is
 * drawn until the user searches, and a search draws at most MAX_MATCHES rows.
 * The name goes into the rename draft like any other setting and takes the
 * plan → execute path with the rest (ADR 0012).
 */
const MAX_MATCHES = 20

const props = defineProps<{
  members: WorksetMember[]
  /** The draft's member records: the names already given. */
  records: RenameDraftMember[]
  disabled: boolean
  saving: boolean
}>()
const emit = defineEmits<{
  /** An empty name clears the folder's own name. */
  save: [memberId: string, name: string]
}>()

// Filtering is cheap; drawing is not. These are built once per record, so a
// keystroke is a loop that stops at MAX_MATCHES (or one Map hit for a full
// work code) and never needs a debounce.
const index = computed(() => {
  const haystack = props.members.map((member) => `${member.rel_path}`.toLowerCase())
  const byCode = new Map<string, WorksetMember[]>()
  for (const member of props.members) {
    const code = codeOf(member.folder_name)
    if (code) byCode.set(code, [...(byCode.get(code) ?? []), member])
  }
  const byId = new Map(props.members.map((member) => [member.member_id, member]))
  return { haystack, byCode, byId }
})

function codeOf(text: string): string | null {
  const match = /rj(\d{6,8})/i.exec(text)
  return match ? `RJ${match[1].padStart(8, '0')}` : null
}

const query = ref('')
const matches = computed(() => {
  const text = query.value.trim().toLowerCase()
  if (!text) return []
  if (/^rj\d{6,8}$/.test(text)) {
    const hit = index.value.byCode.get(codeOf(text) ?? '')
    if (hit) return hit.slice(0, MAX_MATCHES)
  }
  const found: WorksetMember[] = []
  const { haystack } = index.value
  for (let i = 0; i < haystack.length && found.length < MAX_MATCHES; i++) {
    if (haystack[i].includes(text)) found.push(props.members[i])
  }
  return found
})

// A name is spent once a run has renamed the folder: the rename rewrites the
// member's recorded folder name, so the two are equal from then on.
const named = computed(() =>
  props.records.flatMap((record) => {
    const member = index.value.byId.get(record.member_id)
    return record.name && member && member.folder_name !== record.name ? [{ member, name: record.name }] : []
  }),
)
const nameOf = computed(() => new Map(named.value.map((entry) => [entry.member.member_id, entry.name])))

const editing = ref<string | null>(null)
const value = ref('')
const input = ref<HTMLInputElement[] | HTMLInputElement | null>(null)

async function edit(member: WorksetMember) {
  if (props.disabled) return
  editing.value = member.member_id
  value.value = nameOf.value.get(member.member_id) ?? member.folder_name
  await nextTick()
  const el = Array.isArray(input.value) ? input.value[0] : input.value
  el?.focus()
  el?.select()
}

function commit(member: WorksetMember) {
  if (editing.value !== member.member_id) return
  const next = value.value.trim()
  const existing = nameOf.value.get(member.member_id)
  editing.value = null
  // Leaving the current name untouched is not a naming decision.
  if (next === (existing ?? member.folder_name)) return
  emit('save', member.member_id, next)
}
</script>

<template>
  <div data-testid="manual-rename" class="space-y-2 text-xs">
    <input
      v-model="query"
      type="search"
      placeholder="搜索文件夹名或 RJ 号，单独命名"
      class="h-8 w-full rounded-md border border-border bg-background px-2"
      data-testid="manual-rename-search"
      :disabled="disabled"
    />
    <ul v-if="matches.length" class="m-0 list-none divide-y divide-border p-0" data-testid="manual-rename-matches">
      <li v-for="member in matches" :key="member.member_id" class="flex min-h-8 flex-wrap items-center gap-2 py-1">
        <!-- A form, not key handlers: a phone's keyboard submits it with its
             "done" key, and the buttons cover a keyboard-less tap. Leaving the
             field does not save, so a stray tap never renames a folder. -->
        <form
          v-if="editing === member.member_id"
          class="flex w-full flex-wrap items-center gap-2"
          data-testid="manual-rename-form"
          @submit.prevent="commit(member)"
        >
          <input
            ref="input"
            v-model="value"
            enterkeyhint="done"
            autocapitalize="off"
            autocomplete="off"
            class="h-9 min-w-0 basis-full rounded-md border border-border bg-background px-2 font-mono sm:h-7 sm:flex-1 sm:basis-0"
            :aria-label="`${member.folder_name} 的新名称`"
            data-testid="manual-rename-input"
            @keydown.esc.prevent="editing = null"
          />
          <Button type="submit" size="xs" data-testid="manual-rename-save">保存</Button>
          <Button type="button" size="xs" variant="ghost" data-testid="manual-rename-cancel" @click="editing = null">取消</Button>
        </form>
        <template v-else>
          <span class="min-w-0 flex-1 truncate font-mono" :title="member.rel_path">{{ member.rel_path }}</span>
          <span v-if="nameOf.get(member.member_id)" class="truncate font-mono text-[var(--text-secondary)]">
            → {{ nameOf.get(member.member_id) }}
          </span>
          <Button size="xs" variant="ghost" :disabled="disabled" @click="edit(member)">命名</Button>
        </template>
      </li>
    </ul>
    <p v-else-if="query.trim()" class="text-[11px] text-[var(--text-muted)]">没有匹配的文件夹</p>

    <div v-if="named.length" class="space-y-1" data-testid="manual-rename-named">
      <p class="font-semibold text-[var(--text-secondary)]">已单独命名 {{ named.length }}</p>
      <ul class="m-0 list-none p-0">
        <li v-for="entry in named" :key="entry.member.member_id" class="flex items-center gap-2">
          <span class="min-w-0 flex-1 truncate font-mono" :title="entry.member.rel_path">
            {{ entry.member.folder_name }} → {{ entry.name }}
          </span>
          <Button
            size="xs"
            variant="ghost"
            :disabled="disabled || saving"
            :aria-label="`清除 ${entry.member.folder_name} 的单独命名`"
            @click="emit('save', entry.member.member_id, '')"
          >
            清除
          </Button>
        </li>
      </ul>
    </div>
  </div>
</template>
