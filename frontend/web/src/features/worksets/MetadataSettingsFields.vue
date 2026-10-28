<script setup lang="ts">
import type { MetadataSettings } from '@/lib/api/types'
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import SettingsApplyBar from '@/features/settings/SettingsApplyBar.vue'
import SettingsColumn from '@/features/settings/SettingsColumn.vue'
import SettingsField from '@/features/settings/SettingsField.vue'
import SettingsHead from '@/features/settings/SettingsHead.vue'
import SettingsOptions from '@/features/settings/SettingsOptions.vue'
import SettingsSegmented from '@/features/settings/SettingsSegmented.vue'
import SettingsSwitch from '@/features/settings/SettingsSwitch.vue'
import { useTaskSettingsForm } from '@/features/settings/use-task-settings-form'

/**
 * The deployment-wide metadata refresh settings (ADR 0008), edited and applied
 * through the Task settings endpoint, so they work with or without a record.
 */
defineProps<{ collapsible: boolean }>()

const MODES = [
  { value: 'fill_missing', label: '只补缺失', hint: '数据已齐的作品跳过，只在缺封面时补封面。' },
  { value: 'refresh_dynamic', label: '刷新动态数据', hint: '重新获取销量、评分等会变化的数据，不下载封面。' },
  { value: 'refresh_all', label: '全部刷新', hint: '每个作品都重新请求，请求最多。' },
]
const LOCALES = [
  { value: 'ja-jp', label: '日本語' },
  { value: 'zh-cn', label: '简体中文' },
  { value: 'zh-tw', label: '繁體中文' },
]
const DEFAULTS: Required<MetadataSettings> = { mode: 'fill_missing', locale: 'ja-jp', download_cover: false }

const { form, query, saved } = useTaskSettingsForm({
  operation: 'metadata_refresh',
  label: '元数据刷新',
  defaults: DEFAULTS,
  toView: (s): Required<MetadataSettings> => ({ mode: s.mode, locale: s.locale, download_cover: s.download_cover }),
})

// The backend drops the cover download for a dynamic refresh, so the box is
// shown off and says why.
const coverLocked = computed(() => form.value('mode') === 'refresh_dynamic')
const summary = computed(() => {
  if (!saved.value)
    return '设置'
  const label = (options: { value: string, label: string }[], value: string) =>
    options.find(option => option.value === value)?.label ?? value
  const mode = form.value('mode')
  return [
    label(MODES, mode),
    label(LOCALES, form.value('locale')),
    ...(form.value('download_cover') && !coverLocked.value
      ? ['封面']
      : []),
  ].join(' · ')
})
</script>

<template>
  <SettingsColumn :summary="summary" :collapsible="collapsible">
    <p v-if="query.isError.value" class="text-xs text-[var(--danger-ink)]" role="alert">
      设置读取失败：{{ (query.error.value as Error)?.message }}
      <Button size="xs" variant="outline" class="ml-2" @click="query.refetch()">重试</Button>
    </p>
    <p v-else-if="!saved" class="text-xs text-[var(--text-secondary)]">加载中…</p>
    <div v-else>
      <SettingsHead
        scope="所有元数据刷新记录共用这组设置。"
        :can-restore="form.canRestoreDefaults.value"
        :disabled="form.applying.value"
        @restore="form.restoreDefaults()"
      />
      <SettingsField
        id="metadata-mode"
        label="刷新范围"
        :modified="form.modified('mode')"
        :disabled="form.applying.value"
        @undo="form.undo('mode')"
      >
        <SettingsOptions
          data-testid="metadata-mode"
          name="metadata-mode"
          label="刷新范围"
          :options="MODES"
          :value="form.value('mode')"
          :disabled="form.applying.value"
          @change="form.set('mode', $event as MetadataSettings['mode'])"
        />
      </SettingsField>
      <SettingsField
        id="metadata-locale"
        label="标签语言"
        hint="请求作品页时使用的语言版本。"
        :modified="form.modified('locale')"
        :disabled="form.applying.value"
        @undo="form.undo('locale')"
      >
        <SettingsSegmented
          label="标签语言"
          :options="LOCALES"
          :value="form.value('locale')"
          :disabled="form.applying.value"
          data-testid="metadata-locale"
          @change="form.set('locale', $event as MetadataSettings['locale'])"
        />
      </SettingsField>
      <SettingsField
        id="metadata-cover"
        label="封面"
        :modified="form.modified('download_cover')"
        :disabled="form.applying.value || coverLocked"
        @undo="form.undo('download_cover')"
      >
        <SettingsSwitch
          :checked="form.value('download_cover') && !coverLocked"
          :disabled="form.applying.value || coverLocked"
          testid="metadata-cover"
          @change="form.set('download_cover', $event)"
        >
          下载封面
        </SettingsSwitch>
        <p v-if="coverLocked" class="text-[11px] text-[var(--text-secondary)]" data-testid="metadata-cover-reason">
          刷新动态数据只更新销量与评分，不下载封面
        </p>
      </SettingsField>
    </div>
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
