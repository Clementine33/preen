<script setup lang="ts">
import type { ExecutionComponent } from '@/lib/api/types'
import { ChevronRight } from '@lucide/vue'
import { computed } from 'vue'
import { Badge } from '@/components/ui/badge'
import { keepReasonText, PARTITION_TEXT } from '@/features/worksets/plan-readers'

/**
 * One component of a Member Result: its partition, status, operation progress,
 * the stage it stopped at and its error, with the file groups (written, cleaned
 * up, unexecuted, and the files the plan kept with their reasons) folded into a
 * `<details>`. Used by the member execution page only, so the session page
 * never renders per-component DOM.
 */
const props = defineProps<{
  component: ExecutionComponent
  /** The preserved files live under Delete/ in soft mode; hard mode removes them. */
  hardDelete?: boolean
}>()

const COMPONENT_STATUS: Record<ExecutionComponent['status'], { label: string, tone: 'neutral' | 'success' | 'danger' | 'warning' }> = {
  pending: { label: '未执行', tone: 'neutral' },
  succeeded: { label: '已完成', tone: 'success' },
  failed: { label: '失败', tone: 'danger' },
  canceled: { label: '已取消', tone: 'warning' },
}

const STAGE_LABELS: Record<string, string> = {
  precheck: '预检',
  materialize: '生成输出',
  validate: '校验输出',
  commit: '提交',
  remove: '清理旧音频',
}

const removedHint = computed(() => (props.hardDelete ? '已硬删除' : '已清理到 Delete/'))

function filePath(path: string, root: string): string {
  const prefix = `${root.replace(/\/$/, '')}/`
  return path.startsWith(prefix)
    ? path.slice(prefix.length)
    : path
}

function partitionText(partition: string): string {
  return PARTITION_TEXT[partition as keyof typeof PARTITION_TEXT] ?? partition
}
</script>

<template>
  <div class="border-b border-border/60 px-3 py-2.5 last:border-b-0" data-testid="execution-component">
    <div class="flex flex-wrap items-center gap-1.5">
      <span class="text-[11px] font-medium">{{ partitionText(component.partition) }}</span>
      <Badge :tone="COMPONENT_STATUS[component.status].tone">{{ COMPONENT_STATUS[component.status].label }}</Badge>
      <span class="ml-auto text-[11px] tabular-nums text-muted-foreground">
        操作 {{ component.completed_operations }}/{{ component.operations }}
      </span>
      <span v-if="component.stage" class="text-[10px] text-[var(--danger-ink)]">
        停在{{ STAGE_LABELS[component.stage] ?? component.stage }}
      </span>
    </div>

    <p v-if="component.error_code" class="mt-1 text-[11px] text-[var(--danger-ink)]">
      {{ component.error_message || component.error_code }}
    </p>

    <details
      v-if="component.committed.length || component.removed.length || component.remaining.length || component.kept_decisions.length"
      class="group mt-2 border border-[var(--wire-fine)] text-[11px]"
    >
      <summary class="flex cursor-pointer list-none flex-wrap items-center gap-x-2 gap-y-1 rounded-md px-2 py-1.5 text-muted-foreground hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-ring [&::-webkit-details-marker]:hidden">
        <ChevronRight class="size-3 shrink-0 transition-transform group-open:rotate-90" aria-hidden="true" />
        <span>文件明细</span>
        <span v-if="component.committed.length">写入 {{ component.committed.length }}</span>
        <span v-if="component.removed.length">清理 {{ component.removed.length }}</span>
        <span v-if="component.remaining.length">未执行 {{ component.remaining.length }}</span>
        <span v-if="component.kept_decisions.length">保留 {{ component.kept_decisions.length }}</span>
      </summary>
      <dl class="space-y-3 border-t border-border/50 px-2 py-2">
        <template
          v-for="group in [
            { label: '已写入', paths: component.committed, testId: 'execution-committed' },
            { label: removedHint, paths: component.removed, testId: 'execution-removed' },
            { label: '未执行', paths: component.remaining, testId: 'execution-remaining' },
          ]"
          :key="group.testId"
        >
          <div v-if="group.paths.length">
            <dt class="mb-1 text-muted-foreground">{{ group.label }}</dt>
            <dd class="space-y-1" :data-testid="group.testId">
              <span v-for="path in group.paths" :key="path" class="block break-all font-mono leading-relaxed" :title="path">
                {{ filePath(path, component.root_path) }}
              </span>
            </dd>
          </div>
        </template>
        <div v-if="component.kept_decisions.length">
          <dt class="mb-1 text-muted-foreground">保留文件</dt>
          <dd class="space-y-2" data-testid="execution-kept">
            <div v-for="file in component.kept_decisions" :key="file.path">
              <span class="block break-all font-mono leading-relaxed" :title="file.path">{{ filePath(file.path, component.root_path) }}</span>
              <span class="text-[10px] text-muted-foreground">{{ keepReasonText(file.reason_code) }}</span>
            </div>
          </dd>
        </div>
      </dl>
    </details>
    <p v-if="component.inventory_sync_error" class="mt-1 text-[10px] text-[var(--warning-ink)]" data-testid="execution-inventory-warning">
      磁盘结果与媒体库索引未完全同步：{{ component.inventory_sync_error }}
    </p>
  </div>
</template>
