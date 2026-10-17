<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import { usePreferences, useSetTrackFormats } from '@/queries/preferences'

/**
 * The Format Preference editor: the ordered formats a work's track list shows
 * and plays (first one the work ships wins). Every edit saves at once; the
 * server owns the list, so it follows the user across browsers.
 */
const CANDIDATES = ['MP3', 'WAV', 'FLAC', 'M4A', 'OPUS'] as const

const { trackFormats } = usePreferences()
const save = useSetTrackFormats()

const remaining = computed(() => CANDIDATES.filter((format) => !trackFormats.value.includes(format)))

function commit(next: string[]): void {
  save.mutate(next)
}
function move(index: number, delta: number): void {
  const next = [...trackFormats.value]
  ;[next[index], next[index + delta]] = [next[index + delta], next[index]]
  commit(next)
}
function remove(index: number): void {
  commit(trackFormats.value.filter((_, i) => i !== index))
}
function add(format: string): void {
  commit([...trackFormats.value, format])
}
</script>

<template>
  <div data-testid="format-preference">
    <ol v-if="trackFormats.length > 0" class="divide-y divide-[var(--wire-fine)] border-y border-[var(--wire-fine)]">
      <li v-for="(format, index) in trackFormats" :key="format" class="flex items-center gap-1 py-1" :data-testid="`format-${format}`">
        <span class="w-6 text-[11px] text-[var(--text-secondary)]" aria-hidden="true">{{ index + 1 }}</span>
        <span class="flex-1 font-mono text-xs">{{ format }}</span>
        <Button size="xs" variant="ghost" :aria-label="`${format} 上移`" :disabled="index === 0" @click="move(index, -1)">↑</Button>
        <Button size="xs" variant="ghost" :aria-label="`${format} 下移`" :disabled="index === trackFormats.length - 1" @click="move(index, 1)">↓</Button>
        <Button size="xs" variant="ghost" :aria-label="`移除 ${format}`" @click="remove(index)">×</Button>
      </li>
    </ol>
    <p v-else class="text-[11px] text-[var(--text-secondary)]" data-testid="format-empty">未设置：作品显示并播放全部格式。</p>
    <div v-if="remaining.length > 0" class="mt-1.5 flex flex-wrap items-center gap-1" data-testid="format-add">
      <span class="mr-1 text-[11px] text-[var(--text-secondary)]">添加</span>
      <Button v-for="format in remaining" :key="format" size="xs" variant="outline" class="font-mono"
        :aria-label="`添加 ${format}`" :data-testid="`format-add-${format}`" @click="add(format)">{{ format }}</Button>
    </div>
  </div>
</template>
