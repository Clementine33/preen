<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { Badge } from '@/components/ui/badge'
import { useCurrentConversion } from '@/composables/use-operation-context'
import { MEMBER_STATUS } from '@/features/worksets/execution-labels'
import ExecutionComponentRow from '@/features/worksets/ExecutionComponentRow.vue'
import { useApiClient } from '@/lib/api/client'
import { executionMemberQueryOptions } from '@/queries/worksets'

/**
 * One member's Member Result (`.../conversion/:memberId/execution`): its
 * components in frozen order with their file detail. It opens no stream of its
 * own: it re-reads when the shared execution store reports a `member` event for
 * this member (or a fresh snapshot after a reconnect), and the session's
 * terminal sweep re-reads the query once more.
 */
const route = useRoute()
const api = useApiClient()
const libraryId = computed(() => (route.params.libraryId as string) || '')
const memberId = computed(() => (route.params.memberId as string) || '')
const { worksetId, execution, executionView, queries, workspace } = useCurrentConversion(libraryId, computed(() => false))

const executionId = computed(() => executionView.value?.execution_id ?? null)
const memberQuery = useQuery(() => executionMemberQueryOptions(api, worksetId.value, 'conversion', executionId.value, memberId.value))
const view = computed(() => memberQuery.data.value ?? null)

watch(() => execution.store.memberTick, (tick) => {
  if (tick.memberId === null || tick.memberId === memberId.value)
    void memberQuery.refetch()
}, { flush: 'sync' })

const name = computed(() =>
  workspace.workset.value?.members.find(member => member.member_id === memberId.value)?.folder_name ?? memberId.value)
const notInExecution = computed(() => (memberQuery.error.value as { code?: string } | null)?.code === 'MEMBER_NOT_IN_EXECUTION')
const loading = computed(() => queries.operationQuery.isPending.value || queries.executionQuery.isLoading.value || memberQuery.isLoading.value)
const backRoute = computed(() => ({
  name: 'conversion-execution',
  params: { libraryId: libraryId.value },
  query: route.query,
}))
</script>

<template>
  <div class="flex min-h-0 flex-col" data-testid="execution-member-page">
    <p v-if="loading" class="p-3 text-xs text-[var(--text-muted)]">加载中…</p>
    <p
      v-else-if="notInExecution || !executionView"
      class="p-3 text-xs text-[var(--text-muted)]"
      data-testid="execution-member-empty"
    >
      本次执行没有这个文件夹的结果。
    </p>
    <p v-else-if="memberQuery.error.value" class="p-3 text-xs text-[var(--danger-ink)]" role="alert">读取执行结果失败。</p>
    <template v-else-if="view?.member">
      <div class="flex flex-wrap items-center gap-2 border-b border-[var(--wire-strong)] px-4 py-2.5">
        <span class="min-w-0 break-words font-heading text-xs font-semibold" data-testid="execution-member-title" :title="view.member.root_path">{{ name }}</span>
        <Badge :tone="MEMBER_STATUS[view.member.status].tone" data-testid="execution-member-status">{{ MEMBER_STATUS[view.member.status].label }}</Badge>
        <span class="font-mono text-[11px] tabular-nums" data-testid="execution-member-progress">
          组件 {{ view.member.completed_components }}/{{ view.member.total_components }} · 操作 {{ view.member.completed_operations }}/{{ view.member.total_operations }}
        </span>
        <RouterLink :to="backRoute" class="ml-auto text-[11px] underline" data-testid="execution-member-back">返回执行结果</RouterLink>
      </div>
      <ExecutionComponentRow
        v-for="component in view.components ?? []"
        :key="component.component_id"
        :component="component"
        :hard-delete="view.options?.delete_mode === 'hard'"
      />
    </template>
  </div>
</template>
