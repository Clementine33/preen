<script setup lang="ts">
import { Download, Layers, PenLine, X } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import type { OperationType } from '@/lib/api/types'

/**
 * The overview's scope bar (ADR 0001 §1, §2). It belongs to a selection, so
 * the page mounts it only while something is selected — there is never
 * an empty strip above the bottom bar. It sits in the page's own column, so
 * the bottom bar and this bar never overlap.
 *
 * The action lives here, not in a detail column, because it must be reachable
 * at every container tier — a phone has no room for a second column and the
 * scope is chosen on the very page the folders are listed on.
 */
defineProps<{
  selectedCount: number
  loading: boolean
  /** Per operation: entering it replaces the record that is already there. */
  replacing: Record<OperationType, boolean>
  /** Selected folders conversion will skip, because they hold no audio. */
  audioLessCount: number
}>()
const emit = defineEmits<{
  clear: []
  enter: [operation: OperationType]
}>()
</script>

<template>
  <div
    class="flex min-h-14 shrink-0 flex-wrap items-center gap-x-3 gap-y-1 edge-top substrate-panel px-4 py-1.5 rail:px-5"
  >
    <div class="h-5 w-1 shrink-0 bg-[var(--ring)]" aria-hidden="true" />
    <div class="min-w-0">
      <p class="font-heading text-xs font-semibold">处理范围</p>
      <p class="font-mono text-[11px] text-muted-foreground">已选择 {{ selectedCount }} 个文件夹</p>
      <p v-if="audioLessCount > 0" class="text-[10px] text-[var(--warning-ink,var(--text-secondary))]" data-testid="audio-less-note">
        其中 {{ audioLessCount }} 个没有音频，转换会跳过
      </p>
    </div>
    <div class="ml-auto flex shrink-0 items-center gap-2">
      <Button
        data-testid="clear-selection"
        class="h-11 rail:h-7"
        variant="ghost"
        size="sm"
        :disabled="loading"
        @click="emit('clear')"
      >
        <X class="size-3.5" />
        清除
      </Button>
      <Button
        data-testid="enter-rename"
        class="h-11 rail:h-7"
        variant="outline"
        size="sm"
        :disabled="loading"
        @click="emit('enter', 'rename')"
      >
        <PenLine class="size-3.5" />
        {{ replacing.rename ? '替换重命名记录' : '重命名' }}
      </Button>
      <Button
        data-testid="enter-metadata"
        class="h-11 rail:h-7"
        variant="outline"
        size="sm"
        :disabled="loading"
        @click="emit('enter', 'metadata_refresh')"
      >
        <Download class="size-3.5" />
        {{ replacing.metadata_refresh ? '替换元数据记录' : '获取元数据' }}
      </Button>
      <Button
        data-testid="enter-conversion"
        class="h-11 rail:h-7"
        size="sm"
        :disabled="loading"
        @click="emit('enter', 'conversion')"
      >
        <Layers class="size-3.5" />
        {{ loading ? '处理中…' : replacing.conversion ? '替换当前转换记录' : '进入转换' }}
      </Button>
    </div>
  </div>
</template>
