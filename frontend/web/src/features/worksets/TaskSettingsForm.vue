<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import type { MetadataDraftDocument, OperationType, RenameDraftDocument } from '@/lib/api/types'

/**
 * The common settings of a rename or metadata refresh record (docs/api.md
 * §Rename task, §Metadata refresh task). Both are deployment-wide: saving a
 * changed value moves every record of the same kind (ADR 0010 §2).
 */
type Common = RenameDraftDocument['common'] | MetadataDraftDocument['common']

const props = defineProps<{
  operation: OperationType
  common: Common | null
  disabled: boolean
  saving: boolean
}>()
const emit = defineEmits<{ save: [common: Common] }>()

const PLACEHOLDERS = ['{rj}', '{title}', '{circle}', '{voice_actors}', '{language}', '{release_date}']
const MODES = [
  { value: 'fill_missing', label: '只补缺失' },
  { value: 'refresh_dynamic', label: '刷新动态数据（销量、评分等）' },
  { value: 'refresh_all', label: '全部刷新' },
] as const
const LOCALES = [
  { value: 'ja-jp', label: '日本語' },
  { value: 'zh-cn', label: '简体中文' },
  { value: 'zh-tw', label: '繁體中文' },
] as const

const template = ref('')
const mode = ref<MetadataDraftDocument['common']['mode']>('fill_missing')
const locale = ref<MetadataDraftDocument['common']['locale']>('ja-jp')
const downloadCover = ref(true)

// The server copy reseeds the form whenever it changes (a save, another tab).
watch(
  () => props.common,
  (common) => {
    if (!common) return
    if ('template' in common) template.value = common.template
    else {
      mode.value = common.mode
      locale.value = common.locale
      downloadCover.value = common.download_cover
    }
  },
  { immediate: true },
)

// The backend drops cover download for a dynamic refresh; the form says so.
const coverDisabled = computed(() => mode.value === 'refresh_dynamic')

function save() {
  emit(
    'save',
    props.operation === 'rename'
      ? { template: template.value }
      : { mode: mode.value, locale: locale.value, download_cover: !coverDisabled.value && downloadCover.value },
  )
}
</script>

<template>
  <form class="space-y-2" data-testid="task-settings" @submit.prevent="save">
    <template v-if="operation === 'rename'">
      <label class="block text-[11px] text-[var(--text-secondary)]" for="rename-template">命名模板</label>
      <input
        id="rename-template"
        v-model="template"
        class="w-full rounded-md border border-border bg-background px-2 py-1 font-mono text-xs"
        :disabled="disabled"
        data-testid="rename-template"
      />
      <p class="text-[10px] text-[var(--text-muted)]">
        必须且只能包含一个 {rj}。可用占位符：<span class="font-mono">{{ PLACEHOLDERS.join(' ') }}</span>
      </p>
    </template>
    <template v-else>
      <div class="flex flex-wrap items-center gap-3 text-xs">
        <label class="flex items-center gap-1">
          模式
          <select v-model="mode" class="rounded-md border border-border bg-background px-1 py-0.5" :disabled="disabled" data-testid="metadata-mode">
            <option v-for="item in MODES" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
        </label>
        <label class="flex items-center gap-1">
          标签语言
          <select v-model="locale" class="rounded-md border border-border bg-background px-1 py-0.5" :disabled="disabled" data-testid="metadata-locale">
            <option v-for="item in LOCALES" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
        </label>
        <label class="flex items-center gap-1" :title="coverDisabled ? '刷新动态数据不下载封面' : undefined">
          <input v-model="downloadCover" type="checkbox" :disabled="disabled || coverDisabled" data-testid="metadata-cover" />
          下载封面
        </label>
      </div>
    </template>
    <Button type="submit" size="xs" variant="outline" :disabled="disabled || saving" data-testid="save-settings">
      {{ saving ? '保存中…' : '保存设置' }}
    </Button>
  </form>
</template>
