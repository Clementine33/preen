<script setup lang="ts">
import { computed, ref } from 'vue'
import { Button } from '@/components/ui/button'
import Modal from '@/components/ui/modal/Modal.vue'
import { ApiError } from '@/lib/api/client'
import { errorDetails } from '@/lib/api/error'
import type { CleanupCategory, CleanupPreview, CleanupStatus } from '@/lib/api/types'
import { isCleanupActive, useCancelCleanup, usePreviewCleanup, useCleanupRun, useStartCleanup } from '@/queries/cleanup'

/**
 * 数据维护: deployment-wide record cleanup by category. One dialog carries a
 * cleanup through: a read-only total of the selection to confirm against, then
 * the live run and its result. The run belongs to the server, so closing,
 * leaving or refreshing never cancels it; the page only offers a way back to a
 * live run and never shows an earlier run's result. Nothing here lists,
 * selects or deletes individual records.
 */
const CATEGORIES: { id: CleanupCategory; label: string }[] = [
  { id: 'scan_sessions', label: '扫描记录' },
  { id: 'plan_generations', label: '规划记录' },
  { id: 'generation_records', label: '无对应文件的生成记录' },
  { id: 'works', label: '无副本、未入合集且无评分、备注、标签的作品' },
  { id: 'relation_reports', label: '无用途的关系报告' },
  { id: 'source_tags', label: '无引用的来源标签' },
  { id: 'source_voice_actors', label: '无引用的来源声优' },
  { id: 'source_circles', label: '无引用的来源社团' },
]
const LABELS = Object.fromEntries(CATEGORIES.map((c) => [c.id, c.label])) as Record<CleanupCategory, string>
const STATUS_TEXT: Record<CleanupStatus, string> = {
  running: '清理正在运行',
  stopping: '正在停止，当前批次提交后结束',
  stopped: '已按请求停止',
  succeeded: '已完成',
  failed: '失败',
  interrupted: '已中断',
}

const run = useCleanupRun()
const previewMutation = usePreviewCleanup()
const startMutation = useStartCleanup()
const cancelMutation = useCancelCleanup()

const current = computed(() => run.data.value ?? null)
const active = computed(() => isCleanupActive(current.value))
// A failed read keeps the last data, so the run may have moved on unseen.
const unknown = computed(() => run.isError.value)

const selected = ref<CleanupCategory[]>(CATEGORIES.map((c) => c.id))
const preview = ref<CleanupPreview | null>(null)
const previewError = ref('')
const dialog = ref<'confirm' | 'run' | null>(null)
const error = ref('')
// The run started from this dialog; its result is shown once it ends, any other ended run is not.
const own = ref<string | null>(null)
const shown = computed(() => (current.value && (active.value || current.value.run_id === own.value) ? current.value : null))
let seq = 0

const ordered = computed(() => CATEGORIES.filter((c) => selected.value.includes(c.id)))
const total = computed(() => ordered.value.reduce((sum, c) => sum + (preview.value?.candidates[c.id] ?? 0), 0))

// The total is an estimate: execution rechecks every record before deleting it.
// A late answer for a dialog already closed is dropped.
async function open(): Promise<void> {
  const mine = ++seq
  preview.value = null
  previewError.value = ''
  error.value = ''
  dialog.value = 'confirm'
  try {
    const result = await previewMutation.mutateAsync(ordered.value.map((c) => c.id))
    if (mine === seq) preview.value = result
  } catch (failure) {
    if (mine === seq) previewError.value = errorDetails(failure).message
  }
}
function close(): void {
  seq++
  dialog.value = null
}

async function start(): Promise<void> {
  dialog.value = 'run'
  error.value = ''
  own.value = null
  try {
    own.value = (await startMutation.mutateAsync(ordered.value.map((c) => c.id))).run_id
  } catch (failure) {
    // Never resend: ask the server what is running instead.
    void run.refetch()
    if (failure instanceof ApiError && failure.code === 'BUSY') error.value = '当前有其他维护、扫描或任务在运行，未开始。'
    else if (failure instanceof ApiError) error.value = failure.message
    else error.value = '连接中断，无法确认是否已开始。已重新查询状态，请勿重复提交。'
  }
}

async function stop(): Promise<void> {
  if (!current.value) return
  error.value = ''
  try {
    await cancelMutation.mutateAsync(current.value.run_id)
  } catch (failure) {
    void run.refetch()
    error.value = failure instanceof ApiError ? failure.message : '连接中断，停止请求状态未知，已重新查询。'
  }
}

</script>

<template>
  <div class="space-y-4" data-testid="data-maintenance">
    <p class="text-xs leading-relaxed text-[var(--text-secondary)]">自动清理保留最近 7 天的扫描记录和 30 天的规划记录，完整一轮结束后每 24 小时运行一次。</p>

    <p v-if="unknown" role="alert" class="text-xs text-destructive" data-testid="cleanup-unknown">
      无法读取清理状态，当前状态未知。
      <Button size="xs" variant="outline" @click="run.refetch()">重新查询</Button>
    </p>

    <p v-if="active" class="flex items-center gap-2 text-xs" data-testid="cleanup-active">
      清理正在运行
      <Button size="xs" variant="outline" data-testid="cleanup-show" @click="dialog = 'run'">查看进度</Button>
    </p>

    <template v-else>
      <fieldset class="space-y-1">
        <legend class="text-xs font-medium">清理类别</legend>
        <label v-for="c in CATEGORIES" :key="c.id" class="flex min-h-11 items-center gap-2 text-xs rail:min-h-7">
          <input v-model="selected" type="checkbox" :value="c.id" :data-testid="`cleanup-select-${c.id}`" />
          <span class="flex-1">{{ c.label }}</span>
        </label>
      </fieldset>

      <Button size="sm" variant="destructive" :disabled="selected.length === 0 || startMutation.isPending.value" data-testid="cleanup-start" @click="open">
        开始清理
      </Button>
    </template>

    <Modal :open="dialog === 'confirm'" title="确认清理数据库记录" @update:open="close">
      <p v-if="previewError" role="alert" class="text-xs text-destructive" data-testid="cleanup-preview-error">{{ previewError }}</p>
      <p v-else-if="!preview" class="text-xs" data-testid="cleanup-counting">正在统计…</p>
      <p v-else-if="total === 0" class="text-xs" data-testid="cleanup-none">无需清理</p>
      <p v-else class="text-xs leading-relaxed" data-testid="cleanup-confirm-total">将清理 <span class="font-mono">{{ total }}</span> 条记录。删除的记录无法撤销；实际数量以执行时重新检查为准。</p>
      <div class="flex justify-end gap-2">
        <Button variant="ghost" size="sm" @click="close">取消</Button>
        <Button variant="destructive" size="sm" :disabled="!preview || total === 0" data-testid="cleanup-confirm" @click="start">确认清理</Button>
      </div>
    </Modal>

    <Modal :open="dialog === 'run'" title="数据库清理" @update:open="close">
      <p v-if="unknown" role="alert" class="text-xs text-destructive">无法读取清理状态，当前状态未知。</p>
      <p v-if="error" role="alert" class="text-xs text-destructive" data-testid="cleanup-error">{{ error }}</p>
      <section v-if="shown" class="space-y-1.5" aria-live="polite" data-testid="cleanup-run">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-xs font-medium" data-testid="cleanup-status">{{ STATUS_TEXT[shown.status] }}</span>
          <span v-if="active && shown.phase" class="text-[11px] text-[var(--text-secondary)]">阶段：{{ LABELS[shown.phase] ?? shown.phase }}</span>
        </div>
        <p v-if="shown.reason" class="text-[11px] text-[var(--text-secondary)]">{{ shown.reason }}</p>
        <table class="w-full text-[11px]">
          <thead><tr class="text-left text-[var(--text-secondary)]"><th scope="col" class="font-normal">类别</th><th scope="col" class="font-normal">已检查</th><th scope="col" class="font-normal">已删除</th><th scope="col" class="font-normal">已跳过</th></tr></thead>
          <tbody>
            <tr v-for="id in shown.categories" :key="id" :data-testid="`cleanup-count-${id}`">
              <th scope="row" class="font-normal">{{ LABELS[id] }}</th>
              <td class="font-mono">{{ shown.counts[id]?.checked ?? 0 }}</td>
              <td class="font-mono">{{ shown.counts[id]?.deleted ?? 0 }}</td>
              <td class="font-mono">{{ shown.counts[id]?.skipped ?? 0 }}</td>
            </tr>
          </tbody>
        </table>
      </section>
      <p v-else-if="startMutation.isPending.value" class="text-xs">正在开始…</p>
      <div class="flex justify-end gap-2">
        <Button v-if="active" variant="outline" size="sm" :disabled="current?.status === 'stopping' || cancelMutation.isPending.value" data-testid="cleanup-stop" @click="stop">停止</Button>
        <Button variant="ghost" size="sm" data-testid="cleanup-close" @click="close">关闭</Button>
      </div>
    </Modal>
  </div>
</template>
