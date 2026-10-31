<script setup lang="ts">
import type { MetadataTaskResult, OperationType, RenameTaskResult } from '@/lib/api/types'
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { executionMemberQueryOptions } from '@/queries/worksets'

/**
 * One member's observed result on a rename or metadata session, read on demand
 * through the single-member `member_id` read: the session summary carries only
 * counts and exceptions, so the Task's own result (the work code, the folder's
 * old and new path) is fetched only for the member the reader opens.
 */
const props = defineProps<{
  worksetId: string
  operation: OperationType
  executionId: string
  memberId: string
}>()

const api = useApiClient()
const query = useQuery(() => executionMemberQueryOptions(api, props.worksetId, props.operation, props.executionId, props.memberId))
const component = computed(() => query.data.value?.components?.[0] ?? null)

const FAILURES: Record<string, string> = {
  METADATA_UNAVAILABLE: '元数据获取失败',
  COVER_QUEUE_FAILED: '封面排队失败',
}
const text = computed(() => {
  const c = component.value
  if (!c)
    return ''
  if (props.operation === 'metadata_refresh') {
    const result = c.task_result as MetadataTaskResult | undefined
    const reason = c.status === 'failed'
      ? (FAILURES[c.error_code ?? ''] ?? c.error_code ?? '失败')
      : '已刷新'
    return `${result?.rj ?? c.root_path} · ${reason}`
  }
  const facts = c.task_result as RenameTaskResult | undefined
  const status = c.status === 'failed'
    ? facts?.rolled_back
      ? '失败（已恢复原名称）'
      : facts?.renamed ? '失败（目录已改名）' : '失败'
    : facts?.recovered ? '已改名（启动恢复）' : '已改名'
  return `${facts?.source_path ?? c.root_path} → ${facts?.target_path ?? ''} · ${status}`
})
</script>

<template>
  <p class="mt-1 pl-6.5 font-mono text-[11px] text-[var(--text-secondary)]" data-testid="task-member-result">
    <template v-if="query.isPending.value">读取中…</template>
    <template v-else-if="query.isError.value">无法读取该成员的结果</template>
    <template v-else>{{ text }}</template>
  </p>
</template>
