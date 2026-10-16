<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { RouterLink, useRoute } from 'vue-router'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import WorkbenchShell from '@/components/layout/WorkbenchShell.vue'
import WorkbenchNav from '@/features/worksets/WorkbenchNav.vue'
import RenameSettingsFields from '@/features/worksets/RenameSettingsFields.vue'
import MetadataSettingsFields from '@/features/worksets/MetadataSettingsFields.vue'
import FilterTabs from '@/features/worksets/FilterTabs.vue'
import SettingsSection from '@/features/settings/SettingsSection.vue'
import TaskPlanTable from '@/features/worksets/TaskPlanTable.vue'
import ManualRename from '@/features/worksets/ManualRename.vue'
import type { PlanTab } from '@/features/worksets/TaskPlanTable.vue'
import RowList from '@/features/worksets/RowList.vue'
import { useApiClient } from '@/lib/api/client'
import { useWorksetGeneration } from '@/composables/use-workset-generation'
import { useWorksetExecution } from '@/composables/use-workset-execution'
import {
  currentRecordQueryOptions,
  executionCoversQueryOptions,
  executionQueryOptions,
  operationDraftQueryOptions,
  operationQueryOptions,
  operationRevisionDetailQueryOptions,
  saveOperationDraftMutationOptions,
  syncAfterDraftConflict,
} from '@/queries/worksets'
import type {
  MetadataComponent,
  MetadataDraftDocument,
  MetadataTaskResult,
  RenameTaskResult,
  OperationType,
  RenameComponent,
  RenameDraftDocument,
  RevisionDetailResponse,
  TaskPlanPayload,
} from '@/lib/api/types'

/**
 * The rename and metadata refresh section of one library's workbench
 * (ADR 0012). Like the conversion page it addresses the library: the current
 * record of this operation is looked up, and its settings, plan and sessions
 * are what the page shows. Both execute their frozen task units; cover delivery
 * remains an independent metadata queue.
 *
 * Both routes share this component and the router reuses the instance across
 * them, so every query reads `operation` reactively instead of capturing it.
 */
const props = defineProps<{ operation: OperationType }>()
const operation = computed(() => props.operation)
const isMetadata = computed(() => operation.value === 'metadata_refresh')
const label = computed(() => (isMetadata.value ? '获取元数据' : '重命名'))

const route = useRoute()
const api = useApiClient()
const queryClient = useQueryClient()
const generation = useWorksetGeneration()
const execution = useWorksetExecution()

const libraryId = computed(() => (route.params.libraryId as string) || '')
const overviewRoute = computed(() => ({ name: 'workbench-overview' as const, params: { libraryId: libraryId.value } }))

const recordQuery = useQuery(() => currentRecordQueryOptions(api, libraryId.value, operation.value))
const record = computed(() => recordQuery.data.value?.workset ?? null)
const worksetId = computed(() => record.value?.workset_id ?? null)

const operationQuery = useQuery(() => operationQueryOptions(api, worksetId.value, operation.value))
const draftQuery = useQuery(() => operationDraftQueryOptions(api, worksetId.value, operation.value))
const op = computed(() => operationQuery.data.value ?? null)
const planId = computed(() => op.value?.current_revision?.plan_id ?? null)
const revisionQuery = useQuery(() => operationRevisionDetailQueryOptions(api, worksetId.value, operation.value, planId.value))

// The draft and plan payloads belong to their task (ADR 0004): read once here
// in the shapes docs/api.md gives for this operation.
const draftDocument = computed(
  () => (draftQuery.data.value?.document ?? null) as unknown as RenameDraftDocument | MetadataDraftDocument | null,
)
const revision = computed(
  () =>
    (revisionQuery.data.value ?? null) as unknown as RevisionDetailResponse<
      TaskPlanPayload<RenameComponent | MetadataComponent>
    > | null,
)
const components = computed(() => revision.value?.task.payload.components ?? [])

// ---- settings -----------------------------------------------------------

const saveMutation = useMutation(saveOperationDraftMutationOptions(api, queryClient))
const nameError = ref<string | null>(null)
const sessionActive = computed(() => Boolean(op.value?.active_generation || op.value?.active_execution))

async function persistDraft(next: RenameDraftDocument | MetadataDraftDocument) {
  const draft = draftQuery.data.value
  if (!worksetId.value || !draft) return
  nameError.value = null
  try {
    await saveMutation.mutateAsync({
      worksetId: worksetId.value,
      operation: operation.value,
      document: next,
      ifMatchVersion: draft.version,
    })
  } catch (error) {
    const failure = error as { code?: string; message?: string }
    if (failure.code === 'VERSION_CONFLICT') {
      nameError.value = '设置已在别处修改，已重新加载'
      await syncAfterDraftConflict(queryClient, worksetId.value, operation.value)
    } else {
      nameError.value = failure.message ?? '无法保存设置'
    }
  }
}

// A folder's own name is one more record in the same draft; the member's
// exclusion, if the draft holds one, is kept as it is.
const renameRecords = computed(() => (draftDocument.value as RenameDraftDocument | null)?.members ?? [])
function nameFolder(memberId: string, name: string) {
  const document = draftDocument.value as RenameDraftDocument | null
  if (!document) return
  const existing = document.members.find((entry) => entry.member_id === memberId)
  const kept = { member_id: memberId, excluded: existing?.excluded ?? false, ...(name ? { name } : {}) }
  // Names a run has already applied (the folder now has that name) go with this save.
  const folderNames = new Map(record.value?.members.map((member) => [member.member_id, member.folder_name]))
  const rest = document.members
    .filter((entry) => entry.member_id !== memberId)
    .flatMap((entry) => (entry.name && folderNames.get(entry.member_id) === entry.name
      ? entry.excluded ? [{ member_id: entry.member_id, excluded: true }] : []
      : [entry]))
  void persistDraft({ ...document, members: name || kept.excluded ? [...rest, kept] : rest })
}

// ---- planning -----------------------------------------------------------

const GENERATE_CODES: Record<string, string> = {
  NO_ACTIVE_MEMBERS: '所有文件夹都被排除',
  SCAN_IN_PROGRESS: '媒体库正在扫描，等扫描结束后再生成',
  EXECUTION_IN_PROGRESS: '该操作已有执行在进行中',
  BUSY: '当前有其他任务在进行，稍后再生成',
  VERSION_CONFLICT: '操作版本已变化，请刷新后重试',
  ORPHANED_WORKSET: '媒体库已删除：该记录只读',
}
const generateError = ref<string | null>(null)

async function generate() {
  if (!worksetId.value || !op.value) return
  generateError.value = null
  try {
    await generation.start({
      worksetId: worksetId.value,
      operation: operation.value,
      ifMatchVersion: op.value.version,
      idempotencyKey: crypto.randomUUID(),
    })
  } catch (error) {
    const failure = error as { code?: string; message?: string }
    generateError.value = `无法生成：${(failure.code && GENERATE_CODES[failure.code]) ?? failure.message ?? '未知错误'}`
  }
}

// The generation store is shared with conversion: its progress is this page's
// only while it streams this record's session.
const generationProgress = computed(() => {
  const store = generation.store
  const active = op.value?.active_generation
  if (!active) return null
  if (store.status === 'streaming' && store.generationId === active.generation_id) {
    return { done: store.completedRoots, total: store.totalRoots }
  }
  return { done: active.completed_roots, total: active.total_roots }
})

const LOCALE_LABELS: Record<string, string> = { 'ja-jp': '日本語', 'zh-cn': '简体中文', 'zh-tw': '繁體中文' }
const metadataLocaleLabel = computed(() => {
  const locale = (draftDocument.value as MetadataDraftDocument | null)?.common.locale
  return locale ? (LOCALE_LABELS[locale] ?? locale) : '—'
})
const metadataPlanCounts = computed(() => {
  const skip = (components.value as MetadataComponent[]).filter((c) => c.action !== 'refresh').length
  return { run: components.value.length - skip, skip }
})

const planNote = computed(() => {
  const state = op.value?.planning_state
  if (state === 'orphaned') return '媒体库已删除：该记录只读'
  if (state === 'needs_planning') return '设置已改变，需重新生成计划'
  if (op.value?.current_revision?.validation_state === 'stale') return '输入已变化，需重新生成计划'
  return null
})

// ---- execution (frozen task units) -------------------------------------

const EXECUTION_STATES: Record<string, string> = {
  queued: '排队中',
  running: '执行中',
  succeeded: '已完成',
  failed: '失败',
  canceled: '已取消',
  interrupted: '已中断',
}
const executionId = computed(
  () => op.value?.active_execution?.execution_id ?? revision.value?.execution?.execution_id ?? null,
)
const executionQuery = useQuery(() =>
  executionQueryOptions(api, worksetId.value, operation.value, executionId.value),
)
// Like generation, the execution store may be streaming another operation.
const executionView = computed(() => {
  const store = execution.store
  const streamed = store.worksetId === worksetId.value && store.operation === operation.value && store.executionId === executionId.value ? store.view : null
  return store.status === 'streaming' && streamed ? streamed : (executionQuery.data.value ?? streamed)
})
// Every member is a bound work, so the outcome worth listing is the works whose
// fetch failed; non-fatal source issues of a refreshed work are not shown.
const FAILURES: Record<string, string> = {
  METADATA_UNAVAILABLE: '元数据获取失败',
  COVER_QUEUE_FAILED: '封面排队失败',
}
const metadataRows = computed(() =>
  (executionView.value?.components ?? []).flatMap((component) => {
    const failed = component.status === 'failed'
    if (!failed && component.status !== 'succeeded') return []
    const result = component.task_result as MetadataTaskResult | undefined
    if (!failed && ['skip', 'cover'].includes((result as { action?: string } | undefined)?.action ?? '')) return []
    return [{
      key: component.component_id,
      rj: result?.rj ?? component.root_path,
      failed,
      reason: failed ? (FAILURES[component.error_code ?? ''] ?? component.error_code ?? '') : '已刷新',
      message: component.error_message ?? '',
    }]
  }),
)

// A plan of thousands of folders mostly succeeds: successes sit behind their
// own tab, and the failures tab opens first.
const renameResults = computed(() =>
  (executionView.value?.components ?? []).flatMap((component) => {
    const facts = component.task_result as RenameTaskResult | undefined
    if (facts?.skipped) return []
    const failed = component.status === 'failed'
    if (!failed && component.status !== 'succeeded') return []
    const status = failed
      ? facts?.rolled_back ? '失败（已恢复原名称）' : facts?.renamed ? '失败（目录已改名）' : '失败'
      : facts?.recovered ? '已改名（启动恢复）' : '已改名'
    return [{
      key: component.component_id,
      source: facts?.source_path ?? component.root_path,
      target: facts?.target_path ?? '',
      failed,
      status,
      message: component.error_message ?? '',
    }]
  }),
)
const resultRows = computed(() => (isMetadata.value ? metadataRows.value : renameResults.value))
const resultTabs = computed(() => [
  { key: 'failed', label: '失败', count: resultRows.value.filter((row) => row.failed).length },
  { key: 'succeeded', label: '成功', count: resultRows.value.filter((row) => !row.failed).length },
])
const shownResultRows = computed(() => resultRows.value.filter((row) => row.failed === (resultTab.value === 'failed')))
const shownMetadataRows = computed(() => shownResultRows.value as typeof metadataRows.value)
const shownRenameRows = computed(() => shownResultRows.value as typeof renameResults.value)
const renameSkipped = computed(
  () => (executionView.value?.components ?? []).filter((c) => (c.task_result as RenameTaskResult | undefined)?.skipped).length,
)
const metadataSkipped = computed(
  () => (executionView.value?.components ?? []).filter((c) => ['skip', 'cover'].includes((c.task_result as { action?: string } | undefined)?.action ?? '')).length,
)

// ---- the results pane --------------------------------------------------

// Plan tabs list what needs a look: skipped folders first. After a run the
// failures come first. Both reset when a new plan or run replaces the list.
const chosenPlanTab = ref<PlanTab | null>(null)
const resultTab = ref<'failed' | 'succeeded'>('failed')
watch([planId, executionId], () => {
  chosenPlanTab.value = null
  resultTab.value = 'failed'
})
const renameComponents = computed(() => (isMetadata.value ? [] : (components.value as RenameComponent[])))
const planCounts = computed(() => {
  const count = { rename: 0, blocked: 0, noop: 0, unchanged: 0 }
  for (const c of renameComponents.value) {
    if (c.action !== 'noop') count[c.action]++
    else if (c.observed_name) count.noop++
    else count.unchanged++
  }
  return count
})
// Opens on the skipped folders; a plan with none opens on the renames instead
// of an empty tab.
const planTab = computed({
  get: (): PlanTab => chosenPlanTab.value ?? (planCounts.value.blocked ? 'blocked' : 'rename'),
  set: (tab: PlanTab) => (chosenPlanTab.value = tab),
})
const planTabs = computed(() => [
  { key: 'rename', label: '重命名', count: planCounts.value.rename },
  { key: 'blocked', label: '跳过', count: planCounts.value.blocked },
  { key: 'noop', label: '已手动改名', count: planCounts.value.noop },
])

const executeError = ref<string | null>(null)
const canExecute = computed(
  () =>
    Boolean(revision.value) &&
    Boolean(planId.value) &&
    !revision.value?.execution &&
    !sessionActive.value &&
    planNote.value === null,
)

async function execute() {
  if (!worksetId.value || !op.value || !planId.value) return
  executeError.value = null
  try {
    await execution.start({
      worksetId: worksetId.value,
      operation: operation.value,
      planId: planId.value,
      ifMatchVersion: op.value.version,
      idempotencyKey: crypto.randomUUID(),
    })
  } catch (error) {
    const failure = error as { message?: string; details?: string[] }
    executeError.value = `无法执行：${failure.details?.join('；') || failure.message || '未知错误'}`
  }
}

const coversQuery = useQuery(() =>
  executionCoversQueryOptions(api, worksetId.value, isMetadata.value ? executionId.value : null),
)
const coversMutation = execution.coversMutation
watch(() => [worksetId.value, isMetadata.value ? executionId.value : null] as const, ([id, executionId]) => {
  if (id && executionId) execution.observeCovers(id, executionId)
}, { immediate: true })
function changeCovers(action: 'retry' | 'cancel') {
  if (worksetId.value && executionId.value) {
    coversMutation.mutate({ worksetId: worksetId.value, executionId: executionId.value, action })
  }
}

// Reload and route-return recovery: re-attach whichever session is live. The
// stores refuse an attach while they already stream, so this never doubles.
watch(
  () => [worksetId.value, op.value?.active_generation?.generation_id] as const,
  ([id, generationId]) => {
    if (id && generationId) generation.attach(id, operation.value, generationId)
  },
  { immediate: true },
)
watch(
  () => [worksetId.value, op.value?.active_execution?.execution_id] as const,
  ([id, activeId]) => {
    if (id && activeId) execution.attach(id, operation.value, activeId)
  },
  { immediate: true },
)
</script>

<template>
  <WorkbenchShell
    :detail-column="false"
    :context-title="record?.title ?? '…'"
    :context-page="label"
    :context-back-to="overviewRoute"
    context-back-label="概览与成员"
  >
    <template #header>
      <nav aria-label="面包屑" class="flex min-w-0 items-center gap-1.5 text-xs">
        <RouterLink to="/worksets" class="rounded px-1 py-0.5 text-[var(--text-secondary)] hover:bg-muted">工作集</RouterLink>
        <span aria-hidden="true" class="text-[var(--text-muted)]">/</span>
        <RouterLink :to="overviewRoute" class="max-w-40 truncate rounded px-1 py-0.5 hover:bg-muted">
          {{ record?.library?.name ?? '…' }}
        </RouterLink>
        <span aria-hidden="true" class="text-[var(--text-muted)]">/</span>
        <span class="px-1 font-medium" aria-current="page">{{ label }}</span>
      </nav>
    </template>

    <template #nav>
      <WorkbenchNav :library-id="libraryId" />
    </template>

    <template #main="{ tier }">
      <section
        class="flex min-h-0 flex-1"
        :class="tier === 'narrow' ? 'flex-col' : 'flex-row'"
        :data-testid="`task-page-${operation}`"
      >
        <!-- Settings are the Task's, not the record's: they work with no record. -->
        <!-- Like the conversion page's detail column, the settings sit to the
             right of the list; on a phone they fold into a summary above it. -->
        <aside
          class="flex min-h-0 shrink-0 flex-col"
          :class="tier === 'narrow' ? 'max-h-[65%] border-b border-[var(--wire-mid)]' : 'order-last w-[380px] border-l border-border substrate-panel'"
        >
          <component :is="isMetadata ? MetadataSettingsFields : RenameSettingsFields" :key="operation" :collapsible="tier === 'narrow'">
            <SettingsSection v-if="!isMetadata && record" title="单独命名">
              <ManualRename
                :members="record.members"
                :records="renameRecords"
                :disabled="sessionActive || op?.planning_state === 'orphaned'"
                :saving="saveMutation.isPending.value"
                @save="nameFolder"
              />
              <p v-if="nameError" class="mt-2 text-[11px] text-[var(--danger-ink)]" role="alert">{{ nameError }}</p>
            </SettingsSection>
          </component>
        </aside>

        <div
          v-if="!recordQuery.isPending.value && !record"
          class="grid min-w-0 flex-1 place-items-center px-6 text-center"
          data-testid="no-record"
        >
          <div class="max-w-sm">
            <h2 class="font-heading text-sm font-semibold">这个媒体库还没有工作集批次</h2>
            <p class="mt-1 text-xs text-[var(--text-secondary)]">回到概览，勾选文件夹后点击“设为工作集批次”，各 Task 共用这批文件夹。</p>
            <RouterLink :to="overviewRoute" class="mt-3 inline-block text-xs text-[var(--brand-ink)]">返回概览 →</RouterLink>
          </div>
        </div>

        <div v-else class="min-h-0 min-w-0 flex-1 overflow-y-auto" data-testid="results-pane">
          <!-- The breadcrumb and the nav already name the page: the header
               carries only the list's state, its filters and its actions. -->
          <header class="sticky top-0 z-10 space-y-1.5 border-b border-[var(--wire-strong)] bg-background px-3 py-2">
            <div class="flex min-h-7 flex-wrap items-center gap-2">
              <FilterTabs v-if="executionView" v-model="resultTab" :tabs="resultTabs" />
              <FilterTabs v-else-if="!isMetadata && components.length" v-model="planTab" :tabs="planTabs" />
              <span class="font-mono text-[11px] text-[var(--text-secondary)]" data-testid="folder-count">
                {{ record?.members.length ?? 0 }} 个文件夹
              </span>
              <Badge v-if="planNote" tone="warning">{{ planNote }}</Badge>
              <template v-if="executionView">
                <Badge tone="neutral">{{ EXECUTION_STATES[executionView.status] ?? executionView.status }}</Badge>
                <span class="font-mono text-[11px]">{{ executionView.completed_components }}/{{ executionView.total_components }}</span>
                <Badge v-if="(isMetadata ? metadataSkipped : renameSkipped) > 0" tone="warning">
                  跳过 {{ isMetadata ? metadataSkipped : renameSkipped }} 个
                </Badge>
              </template>
              <div class="ml-auto flex items-center gap-2">
                <template v-if="generationProgress">
                  <span class="font-mono text-[11px]">生成中 {{ generationProgress.done }}/{{ generationProgress.total }}</span>
                  <Button
                    size="xs"
                    variant="ghost"
                    @click="generation.cancel(worksetId!, operation, op!.active_generation!.generation_id)"
                  >
                    取消
                  </Button>
                </template>
                <Button
                  v-if="op?.active_execution && executionView"
                  size="xs"
                  variant="ghost"
                  :disabled="execution.store.canceling"
                  @click="execution.cancel(worksetId!, operation, executionView.execution_id)"
                >
                  {{ execution.store.canceling ? '取消中…' : '取消' }}
                </Button>
                <Button v-if="!generationProgress" size="xs" :variant="canExecute ? 'outline' : 'default'" :disabled="!op || sessionActive" data-testid="generate" @click="generate">
                  {{ planId ? '重新生成计划' : '生成计划' }}
                </Button>
                <Button v-if="canExecute" size="xs" data-testid="execute" @click="execute">执行</Button>
              </div>
            </div>
            <p v-if="!isMetadata && !executionView && planCounts.unchanged" class="text-[11px] text-[var(--text-secondary)]" data-testid="plan-unchanged">
              {{ planCounts.unchanged }} 个名称不变（未显示）
            </p>
            <p v-if="generateError" class="text-[11px] text-[var(--danger-ink)]" role="alert">{{ generateError }}</p>
            <p v-if="executeError" class="text-[11px] text-[var(--danger-ink)]" role="alert">{{ executeError }}</p>
            <p v-if="executionView?.error_message" class="text-[11px] text-[var(--danger-ink)]">{{ executionView.error_message }}</p>
          </header>

          <div class="px-3 pb-4" :data-testid="executionView ? 'execution' : 'plan'">
            <template v-if="executionView">
              <p v-if="!shownResultRows.length" class="px-2 py-3 text-xs text-[var(--text-secondary)]">
                {{ resultTab === 'failed' ? '没有失败项。' : '还没有成功项。' }}
              </p>
              <RowList v-else-if="isMetadata" :items="shownMetadataRows" :item-key="(row) => row.key" testid="execution-failures">
                <template #default="{ item }">
                  <span class="font-mono">{{ item.rj }}</span>
                  <span class="truncate text-[var(--text-secondary)]" :title="item.message">{{ item.message }}</span>
                  <span class="text-[11px]" :class="item.failed ? 'text-[var(--danger-ink)]' : 'text-[var(--text-secondary)]'">{{ item.reason }}</span>
                </template>
              </RowList>
              <RowList v-else :items="shownRenameRows" :item-key="(row) => row.key" testid="rename-results">
                <template #default="{ item }">
                  <span class="truncate" :title="item.source">{{ item.source }}</span>
                  <span class="truncate" :title="item.target">{{ item.target }}</span>
                  <span class="text-[11px]" :title="item.message">{{ item.status }}<span v-if="item.message" class="block text-[var(--danger-ink)]">{{ item.message }}</span></span>
                </template>
              </RowList>
              <div v-if="isMetadata && coversQuery.data.value" class="mt-3 flex flex-wrap items-center gap-2 text-[11px]" data-testid="covers">
                <span class="font-semibold text-[var(--text-secondary)]">封面</span>
                <span>完成 {{ coversQuery.data.value.counts.succeeded }}</span>
                <span>等待 {{ coversQuery.data.value.counts.pending + coversQuery.data.value.counts.running }}</span>
                <span>跳过 {{ coversQuery.data.value.counts.skipped }}</span>
                <span>失败 {{ coversQuery.data.value.counts.failed }}</span>
                <span>已停止 {{ coversQuery.data.value.counts.canceled }}</span>
                <Button
                  v-if="coversQuery.data.value.counts.failed + coversQuery.data.value.counts.canceled > 0"
                  size="xs"
                  variant="outline"
                  :disabled="coversMutation.isPending.value"
                  @click="changeCovers('retry')"
                >
                  重试
                </Button>
                <Button
                  v-if="coversQuery.data.value.counts.pending + coversQuery.data.value.counts.running > 0"
                  size="xs"
                  variant="ghost"
                  :disabled="coversMutation.isPending.value"
                  @click="changeCovers('cancel')"
                >
                  停止
                </Button>
                <span v-if="coversMutation.error.value" role="alert" class="text-[var(--danger-ink)]">{{ coversMutation.error.value.message }}</span>
              </div>
            </template>
            <template v-else>
              <p v-if="!planId && !generationProgress" class="py-3 text-xs text-[var(--text-secondary)]">尚未生成计划。</p>
              <p v-else-if="isMetadata" class="py-3 text-xs text-[var(--text-secondary)]" data-testid="metadata-plan-count">
                标签语言 {{ metadataLocaleLabel }} · 将刷新 {{ metadataPlanCounts.run }} 个 · 跳过 {{ metadataPlanCounts.skip }} 个
              </p>
              <TaskPlanTable v-else-if="components.length" :components="renameComponents" :members="record?.members" :tab="planTab" />
            </template>
          </div>
        </div>
      </section>
    </template>
  </WorkbenchShell>
</template>
