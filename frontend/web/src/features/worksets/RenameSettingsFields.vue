<script setup lang="ts">
import { computed, ref } from 'vue'
import { Button } from '@/components/ui/button'
import SettingsApplyBar from '@/features/settings/SettingsApplyBar.vue'
import SettingsColumn from '@/features/settings/SettingsColumn.vue'
import SettingsField from '@/features/settings/SettingsField.vue'
import SettingsHead from '@/features/settings/SettingsHead.vue'
import SettingsSwitch from '@/features/settings/SettingsSwitch.vue'
import { useTaskSettingsForm } from '@/features/settings/use-task-settings-form'

/**
 * The deployment-wide rename settings (ADR 0008), edited and applied through
 * the Task settings endpoint, so they work with or without a record. The
 * default slot adds record-bound sections below them (单独命名).
 */
defineProps<{ collapsible: boolean }>()

const PLACEHOLDERS = [
  '{rj}',
  '{title}',
  '{circle}',
  '{voice_actors}',
  '{language}',
  '{release_date}',
]
interface View {
  template: string
  strip_bracketed_title: boolean
  strip_tilde_title: boolean
  strip_book_title: boolean
  voice_actor_separator: string
}
const DEFAULTS: View = {
  template: '[{rj}] {title}',
  strip_bracketed_title: false,
  strip_tilde_title: false,
  strip_book_title: false,
  voice_actor_separator: '',
}
const STRIPS = [
  {
    key: 'strip_bracketed_title',
    id: 'rename-strip-bracketed',
    label: '去掉【…】',
    mark: '【】',
  },
  {
    key: 'strip_tilde_title',
    id: 'rename-strip-tilde',
    label: '去掉 ～…～',
    mark: '～～',
  },
  {
    key: 'strip_book_title',
    id: 'rename-strip-book',
    label: '去掉《…》',
    mark: '《》',
  },
] as const

const { form, query, saved } = useTaskSettingsForm({
  operation: 'rename',
  label: '重命名',
  defaults: DEFAULTS,
  toView: (s): View => ({
    template: s.template,
    strip_bracketed_title: s.strip_bracketed_title ?? false,
    strip_tilde_title: s.strip_tilde_title ?? false,
    strip_book_title: s.strip_book_title ?? false,
    voice_actor_separator: s.voice_actor_separator ?? '',
  }),
})

const template = computed(() => form.value('template') ?? '')
const summary = computed(() => {
  if (!saved.value)
    return '设置'
  const marks = STRIPS.filter(strip => form.value(strip.key)).map(strip => `移除${strip.mark}`)
  return [template.value, ...marks].join(' · ')
})
const stripsModified = computed(() => STRIPS.some(strip => form.modified(strip.key)))
function undoStrips() {
  for (const strip of STRIPS) form.undo(strip.key)
}

// A placeholder chip inserts at the caret, so the template is built without
// retyping braces.
const templateInput = ref<HTMLInputElement | null>(null)
function insertPlaceholder(placeholder: string) {
  const input = templateInput.value
  const at = input?.selectionStart ?? template.value.length
  const to = input?.selectionEnd ?? at
  form.set('template', template.value.slice(0, at) + placeholder + template.value.slice(to))
  requestAnimationFrame(() => {
    input?.focus()
    input?.setSelectionRange(at + placeholder.length, at + placeholder.length)
  })
}
</script>

<template>
  <SettingsColumn :summary="summary" :collapsible="collapsible">
    <p v-if="query.isError.value" class="text-xs text-[var(--danger-ink)]" role="alert">
      设置读取失败：{{ (query.error.value as Error)?.message }}
      <Button size="xs" variant="outline" class="ml-2" @click="query.refetch()">重试</Button>
    </p>
    <p v-else-if="!saved" class="text-xs text-[var(--text-secondary)]">加载中…</p>
    <template v-else>
      <div>
        <SettingsHead
          scope="所有重命名记录共用这组设置。"
          :can-restore="form.canRestoreDefaults.value"
          :disabled="form.applying.value"
          @restore="form.restoreDefaults()"
        />
        <SettingsField
          id="rename-template"
          label="命名模板"
          hint="必须且只能包含一个 {rj}。点占位符插入到光标处。"
          :modified="form.modified('template')"
          :disabled="form.applying.value"
          @undo="form.undo('template')"
        >
          <input
            ref="templateInput"
            :value="template"
            class="field-input w-full font-mono"
            spellcheck="false"
            :disabled="form.applying.value"
            aria-label="命名模板"
            data-testid="rename-template"
            @input="form.set('template', ($event.target as HTMLInputElement).value)"
          >
          <div class="flex flex-wrap gap-1" role="group" aria-label="插入占位符">
            <button
              v-for="placeholder in PLACEHOLDERS"
              :key="placeholder"
              type="button"
              class="chip"
              :disabled="form.applying.value"
              @click="insertPlaceholder(placeholder)"
            >
              {{ placeholder }}
            </button>
          </div>
        </SettingsField>
        <SettingsField
          v-if="template.includes('{voice_actors}')"
          id="rename-actor-separator"
          label="声优分隔符"
          hint="留空为“、”"
          :modified="form.modified('voice_actor_separator')"
          :disabled="form.applying.value"
          @undo="form.undo('voice_actor_separator')"
        >
          <input
            :value="form.value('voice_actor_separator')"
            maxlength="16"
            placeholder="、"
            class="field-input w-24 font-mono"
            :disabled="form.applying.value"
            aria-label="声优分隔符"
            data-testid="rename-actor-separator"
            @input="form.set('voice_actor_separator', ($event.target as HTMLInputElement).value)"
          >
        </SettingsField>
        <SettingsField
          id="rename-strips"
          label="标题过滤"
          hint="只作用于 {title}，去掉整段括号及其内容。"
          :modified="stripsModified"
          :disabled="form.applying.value"
          @undo="undoStrips"
        >
          <div class="flex flex-col gap-2">
            <SettingsSwitch
              v-for="strip in STRIPS"
              :key="strip.key"
              :checked="form.value(strip.key)"
              :disabled="form.applying.value"
              :testid="strip.id"
              @change="form.set(strip.key, $event)"
            >
              {{ strip.label }}
            </SettingsSwitch>
          </div>
        </SettingsField>
      </div>
      <slot />
    </template>
    <template #bar>
      <SettingsApplyBar
        v-if="form.dirty.value"
        :applying="form.applying.value"
        :error="form.error.value"
        @apply="form.apply()"
        @discard="form.discard()"
      />
    </template>
  </SettingsColumn>
</template>
