<script setup lang="ts">
import type { DesiredProfile, OverrideUnit } from '@/lib/api/types'
import {
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
  TagsInputRoot,
} from 'reka-ui'
import { computed } from 'vue'
import SettingsSegmented from '@/features/settings/SettingsSegmented.vue'
import { cloneProfile } from '@/features/worksets/plan-readers'

/**
 * The editable value of one setting group. Shared by the common settings form
 * and the member/batch override editor so the two entry points always offer the
 * same fields — only what they write differs (the common value vs an override).
 */
const props = defineProps<{
  unit: OverrideUnit
  /** Human name of the group, used in accessible names. */
  label: string
  value: unknown
  disabled?: boolean
  /** Distinguishes the two entry points in testids (common | override). */
  scope: string
}>()

const emit = defineEmits<{ change: [value: unknown] }>()

const MODES = [
  { value: 'available_sources', label: '可用源' },
  { value: 'strict', label: '严格' },
]

// An absent lane is not "do not generate": the declared profile is the
// partition's final shape, so an undeclared output is removed where one exists.
const LOSSLESS_CODECS = [
  { value: '', label: '不需要' },
  { value: 'wav', label: 'WAV' },
  { value: 'flac', label: 'FLAC' },
]
const ENCODED_CODECS = [
  { value: '', label: '不需要' },
  { value: 'mp3', label: 'MP3' },
  { value: 'aac', label: 'AAC' },
  { value: 'opus', label: 'Opus' },
]

/**
 * The encoded lane's named shortcuts. A preset writes one codec and bitrate
 * pair — the same value the manual controls produce — so nothing new is stored
 * and any pair can still be composed by hand; a pair that matches a preset
 * simply lights it up.
 */
const PRESETS = [
  {
    value: 'opus-160',
    codec: 'opus',
    bitrate: 160,
    label: 'Opus 160',
  },
  {
    value: 'aac-256',
    codec: 'aac',
    bitrate: 256,
    label: 'AAC 256',
  },
  {
    value: 'mp3-320',
    codec: 'mp3',
    bitrate: 320,
    label: 'MP3 320',
  },
]

const ROW_LABEL = 'flex h-[30px] items-center text-[11px] text-[var(--text-secondary)]'

const profile = computed<DesiredProfile>(() => cloneProfile(props.value))
const tagList = computed<string[]>(() => (props.value as string[] | undefined) ?? [])

// A tag ends at a comma, with any surrounding spaces swallowed so "," and ", "
// both commit the same tag. Reka keeps a pasted list split the same way.
const TAG_DELIMITER = /\s*,\s*/

function setMode(mode: string) {
  emit('change', mode)
}

function setTags(next: string[]) {
  emit(
    'change',
    next.map(tag => tag.trim()).filter(tag => tag.length > 0),
  )
}

function setCodec(lane: 'lossless' | 'encoded', codec: string) {
  const next = cloneProfile(props.value)
  if (codec === '') {
    delete next[lane]
  }
  else {
    const seeded = {
      ...(next[lane] ?? { codec: '' }),
      codec,
    }
    // An encoded output is only valid with its bitrate; seeding the displayed
    // one keeps a fresh selection from being saved without quality.
    if (lane === 'encoded' && !seeded.quality) {
      seeded.quality = { kind: 'bitrate', bitrate: bitrate.value }
    }
    next[lane] = seeded
  }
  emit('change', next)
}

function setBitrate(bitrate: number) {
  const next = cloneProfile(props.value)
  if (!next.encoded)
    return
  next.encoded = {
    ...next.encoded,
    quality: { kind: 'bitrate', bitrate },
  }
  emit('change', next)
}

const losslessCodec = computed(() => profile.value.lossless?.codec ?? '')
const encodedCodec = computed(() => profile.value.encoded?.codec ?? '')
const bitrate = computed(() => profile.value.encoded?.quality?.bitrate ?? 320)

/** Which preset the stored pair is, if any: the shortcut lights up, nothing more. */
const matchedPreset = computed(
  () =>
    PRESETS.find(preset => preset.codec === encodedCodec.value && preset.bitrate === bitrate.value)?.value ?? null,
)

function applyPreset(preset: (typeof PRESETS)[number]) {
  const next = cloneProfile(props.value)
  next.encoded = {
    codec: preset.codec,
    quality: { kind: 'bitrate', bitrate: preset.bitrate },
  }
  emit('change', next)
}
</script>

<template>
  <div class="space-y-2">
    <SettingsSegmented
      v-if="unit === 'mode'"
      :label="`${label} 转换模式`"
      :options="MODES"
      :value="(value as string) ?? 'strict'"
      :disabled="disabled"
      :data-testid="`${scope}-${unit}`"
      @change="setMode($event)"
    />

    <TagsInputRoot
      v-else-if="unit === 'no_effect_tags' || unit === 'other_tags'"
      :model-value="tagList"
      :disabled="disabled"
      :delimiter="TAG_DELIMITER"
      :add-on-paste="true"
      class="tags w-full"
      :data-testid="`${scope}-${unit}`"
      @update:model-value="setTags"
    >
      <TagsInputItem v-for="(tag, index) in tagList" :key="`${index}-${tag}`" :value="tag" class="tag" lang="ja">
        <TagsInputItemText />
        <TagsInputItemDelete :aria-label="`删除标签 ${tag}`">×</TagsInputItemDelete>
      </TagsInputItem>
      <TagsInputInput
        class="h-5 min-w-[8ch] flex-1 bg-transparent px-1 text-xs outline-none placeholder:text-[var(--text-muted)]"
        :aria-label="`${label} 分类标签`"
        :data-testid="`${scope}-${unit}-input`"
        placeholder="输入后按回车或逗号"
      />
    </TagsInputRoot>

    <!-- One row per control, each label boxed to the 30px control height so
         label and control share a centre line. -->
    <div v-else class="grid grid-cols-[2.5rem_minmax(0,1fr)] items-start gap-x-2 gap-y-1.5">
      <span :class="ROW_LABEL">无损</span>
      <SettingsSegmented
        mono
        :label="`${label} 无损目标`"
        :options="LOSSLESS_CODECS"
        :value="losslessCodec"
        :disabled="disabled"
        :data-testid="`${scope}-${unit}-lossless`"
        @change="setCodec('lossless', $event)"
      />
      <span :class="ROW_LABEL">编码</span>
      <SettingsSegmented
        mono
        :label="`${label} 编码目标`"
        :options="ENCODED_CODECS"
        :value="encodedCodec"
        :disabled="disabled"
        :data-testid="`${scope}-${unit}-encoded`"
        @change="setCodec('encoded', $event)"
      />
      <template v-if="encodedCodec !== ''">
        <span :class="ROW_LABEL">码率</span>
        <label class="flex h-[30px] items-center gap-1 text-[11px] text-[var(--text-secondary)]">
          <input
            type="number"
            min="1"
            class="field-input w-16 font-mono"
            :value="bitrate"
            :disabled="disabled"
            :aria-label="`${label} 码率`"
            :data-testid="`${scope}-${unit}-bitrate`"
            @change="setBitrate(Number(($event.target as HTMLInputElement).value))"
          >
          kbps
        </label>
      </template>
      <!-- Shortcuts for the three pairs this app is run with; the manual codec
           and bitrate above stay the way to compose anything else. -->
      <span :class="ROW_LABEL">预设</span>
      <span class="flex min-h-[30px] flex-wrap items-center gap-1" :data-testid="`${scope}-${unit}-presets`">
        <button
          v-for="preset in PRESETS"
          :key="preset.value"
          type="button"
          class="chip"
          :disabled="disabled"
          :aria-pressed="matchedPreset === preset.value"
          :title="`${preset.codec.toUpperCase()} ${preset.bitrate} kbps`"
          :data-testid="`${scope}-${unit}-preset-${preset.value}`"
          @click="applyPreset(preset)"
        >
          {{ preset.label }}
        </button>
      </span>
    </div>
    <p
      v-if="(unit === 'matched' || unit === 'unmatched') && !profile.lossless && !profile.encoded"
      class="text-[11px] text-[var(--warning-ink)]"
      data-testid="empty-profile-warning"
    >
      两者都不需要：该分类下的音频将被移除，去向由「旧音频处理」决定。
    </p>
    <p v-if="unit === 'no_effect_tags' && tagList.length === 0" role="status" class="text-[11px] text-[var(--warning-ink)]" :data-testid="`${scope}-no-effect-empty-warning`">
      无音效匹配为空：可以保存草稿，生成计划前需要至少一个标签。
    </p>
    <p v-if="unit === 'other_tags' && tagList.some((tag) => /[/\\]/.test(tag))" role="alert" class="text-[11px] text-[var(--danger-ink)]">
      其他版本标签只能匹配目录名称，不能包含 / 或 \。
    </p>
  </div>
</template>
