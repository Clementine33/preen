<script setup lang="ts">
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Button } from '@/components/ui/button'
import { defaultDraftValues } from '@/features/worksets/draft-defaults'
import SettingsApplyBar from '@/features/settings/SettingsApplyBar.vue'
import SettingsField from '@/features/settings/SettingsField.vue'
import SettingsHead from '@/features/settings/SettingsHead.vue'
import SettingsOptions from '@/features/settings/SettingsOptions.vue'
import SettingsSection from '@/features/settings/SettingsSection.vue'
import { useTaskSettingsForm } from '@/features/settings/use-task-settings-form'
import UnitFields from '@/features/worksets/UnitFields.vue'
import { useApiClient } from '@/lib/api/client'
import type { ConversionSettings, DeleteMode, OverrideUnit } from '@/lib/api/types'
import { classifierTagLibraryQueryOptions } from '@/queries/worksets'

/**
 * Common conversion settings. They belong to the deployment, not to a record:
 * the page reads and writes them through the Task settings endpoint, so it
 * works in a library that has no record yet. Applying reaches every
 * conversion record and every record created later (ADR 0008). Per-folder
 * overrides are edited elsewhere and keep saving on their own.
 */
const api = useApiClient()

// 恢复默认 restores the deployment's literal tags, so the form needs them.
const { data: tagLibrary } = useQuery(classifierTagLibraryQueryOptions(api))

/** The settings as the form reads them: the server omits empty mode, tags and delete mode. */
type View = Required<Pick<ConversionSettings, 'mode' | 'no_effect_tags' | 'other_tags' | 'matched' | 'unmatched' | 'other' | 'delete_mode'>>
const defaults = computed<View | undefined>(() =>
  tagLibrary.value
    ? (defaultDraftValues(tagLibrary.value.default_no_effect_tags, tagLibrary.value.default_other_tags) as View)
    : undefined,
)

const { form, query: settingsQuery, saved } = useTaskSettingsForm({
  operation: 'conversion',
  label: '转换',
  defaults,
  toView: (s): View => ({
    mode: s.mode ?? 'available_sources',
    no_effect_tags: s.no_effect_tags ?? [],
    other_tags: s.other_tags ?? [],
    matched: s.matched,
    unmatched: s.unmatched,
    other: s.other ?? s.unmatched,
    delete_mode: s.delete_mode ?? 'soft',
  }),
})

const FIELDS: { unit: OverrideUnit; label: string; hint: string }[] = [
  { unit: 'mode', label: '转换模式', hint: '' },
  { unit: 'no_effect_tags', label: '无音效匹配', hint: '按完整相对路径做字面匹配，不区分大小写' },
  { unit: 'other_tags', label: '其他版本目录', hint: '只匹配目录名称，不区分大小写，留空不隔离。' },
  { unit: 'matched', label: '无音效目标', hint: '命中无音效规则的音频要保留或生成的目标。' },
  { unit: 'unmatched', label: '有音效目标', hint: '未命中无音效规则的音频要保留或生成的目标。' },
  { unit: 'other', label: '其他版本目标', hint: '各版本目录中的音频使用的编码组合目标。' },
]
const SECTIONS = [
  { title: '匹配规则', units: ['mode', 'no_effect_tags', 'other_tags'] as OverrideUnit[] },
  { title: '目标格式', units: ['matched', 'unmatched', 'other'] as OverrideUnit[] },
]
const fieldOf = (unit: OverrideUnit) => FIELDS.find((field) => field.unit === unit)!

const modeHint = computed(() =>
  form.value('mode') === 'strict'
    ? '严格模式要求最终格式完全符合设置；需要转换但缺少合格无损源时阻塞整个组件。'
    : '可用源模式按音轨处理；没有合格无损源时保留原轨，并标记目标未满足。',
)

const DELETE_MODES: { value: DeleteMode; label: string; hint: string; danger?: boolean }[] = [
  { value: 'soft', label: '软删除', hint: '旧文件移动到库根目录下的 Delete/（保留各文件夹的原相对路径），可手动恢复。' },
  { value: 'hard', label: '硬删除', hint: '旧文件直接从磁盘移除，本应用无法恢复。', danger: true },
]
</script>

<template>
  <div class="mx-auto flex min-h-full max-w-2xl flex-col" data-testid="conversion-settings">
    <div class="flex-1 p-3">
      <p v-if="settingsQuery.isError.value" class="text-xs text-[var(--danger-ink)]" role="alert">
        设置读取失败：{{ (settingsQuery.error.value as Error)?.message }}
        <Button size="xs" variant="outline" class="ml-2" @click="settingsQuery.refetch()">重试</Button>
      </p>
      <p v-else-if="!saved" class="text-xs text-[var(--text-secondary)]">加载中…</p>

      <div v-else class="space-y-4" data-testid="common-settings-form">
        <SettingsHead
          scope="这些设置属于整个应用：应用后写入全部转换记录，之后新建的记录也从这里起步。"
          :can-restore="form.canRestoreDefaults.value"
          :disabled="form.applying.value"
          @restore="form.restoreDefaults()"
        />
        <SettingsSection v-for="section in SECTIONS" :key="section.title" :title="section.title">
          <SettingsField
            v-for="unit in section.units"
            :id="`common-group-${unit}`"
            :key="unit"
            :label="fieldOf(unit).label"
            :hint="unit === 'mode' ? modeHint : fieldOf(unit).hint"
            :modified="form.modified(unit)"
            :disabled="form.applying.value"
            :data-unit="unit"
            @undo="form.undo(unit)"
          >
            <UnitFields
              :unit="unit"
              :label="fieldOf(unit).label"
              scope="common"
              :value="form.value(unit)"
              :disabled="form.applying.value"
              @change="form.set(unit, $event as never)"
            />
          </SettingsField>
        </SettingsSection>

        <SettingsSection title="旧音频">
          <SettingsField
            id="common-group-delete_mode"
            label="旧音频处理"
            :modified="form.modified('delete_mode')"
            :disabled="form.applying.value"
            @undo="form.undo('delete_mode')"
          >
            <SettingsOptions
              name="common-delete-mode"
              label="旧音频处理"
              :options="DELETE_MODES"
              :value="form.value('delete_mode')"
              :disabled="form.applying.value"
              @change="form.set('delete_mode', $event as DeleteMode)"
            />
            <p
              v-if="form.value('delete_mode') === 'hard'"
              class="text-[11px] text-[var(--danger-ink)]"
              role="alert"
              data-testid="common-hard-delete-warning"
            >
              硬删除不可恢复：被替换和被清理的旧音频文件将被永久移除。
            </p>
          </SettingsField>
        </SettingsSection>
      </div>
    </div>

    <div v-if="form.dirty.value" class="sticky bottom-0">
      <SettingsApplyBar
        :applying="form.applying.value"
        :error="form.error.value"
        @apply="form.apply()"
        @discard="form.discard()"
      />
    </div>
  </div>
</template>
