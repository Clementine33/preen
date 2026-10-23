<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import Modal from '@/components/ui/modal/Modal.vue'
import { ApiError } from '@/lib/api/client'
import { errorDetails } from '@/lib/api/error'
import type { CleanupCategory, CleanupPreview, CleanupStatus } from '@/lib/api/types'
import { isCleanupActive, useCancelCleanup, usePreviewCleanup, useCleanupRun, useStartCleanup } from '@/queries/cleanup'

/**
 * 数据维护: deployment-wide record cleanup by category. The page checks
 * (read-only summary), confirms, starts and stops; the run belongs to the
 * server, so leaving or refreshing never cancels it and returning shows its
 * state. Nothing here lists, selects or deletes individual records.
 */
const CATEGORIES: { id: CleanupCategory; label: string }[] = [
  { id: 'scan_sessions', label: '扫描记录' },
  { id: 'plan_generations', label: '规划记录' },
  { id: 'cover_downloads', label: '无引用的已完成封面下载记录' },
  { id: 'works', label: '无副本且无个人资料的作品' },
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
const checking = ref(false)
const confirming = ref(false)
const error = ref('')
let seq = 0

// A changed selection invalidates the summary; a late answer for an older one is dropped.
watch(selected, () => { seq++; preview.value = null; checking.value = false; error.value = '' })

const ordered = computed(() => CATEGORIES.filter((c) => selected.value.includes(c.id)))
const total = computed(() => ordered.value.reduce((sum, c) => sum + (preview.value?.candidates[c.id] ?? 0), 0))

async function check(): Promise<void> {
  const mine = ++seq
  error.value = ''
  checking.value = true
  try {
    const result = await previewMutation.mutateAsync(ordered.value.map((c) => c.id))
    if (mine === seq) preview.value = result
  } catch (failure) {
    if (mine === seq) error.value = errorDetails(failure).message
  } finally {
    if (mine === seq) checking.value = false
  }
}

async function start(): Promise<void> {
  confirming.value = false
  error.value = ''
  try {
    await startMutation.mutateAsync(ordered.value.map((c) => c.id))
    preview.value = null
  } catch (failure) {
    // Never resend: ask the server what is running instead.
    void run.refetch()
    if (failure instanceof ApiError && failure.code === 'BUSY') error.value = '当前有其他维护、扫描或任务在运行，未开始。'
    else if (failure instanceof ApiError) error.value = failure.message
    else error.value = '连接中断，无法确认是否已开始。已重新查询状态，请先查看下方状态，不要重复提交。'
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

const rules = computed(() => ordered.value.flatMap((c) => {
  const days = preview.value?.rules[c.id]?.retention_days ?? 0
  return days > 0 ? [{ id: c.id, text: `${c.label}：保留 ${days} 天` }] : []
}))
</script>

<template>
  <div class="space-y-4" data-testid="data-maintenance">
    <div class="space-y-1.5 text-xs leading-relaxed text-[var(--text-secondary)]">
      <p>只删除数据库记录，不删除媒体、作品目录、封面、缩略图或回收目录文件。个人资料和离线副本受保护。</p>
      <p>被清理的作品重新收录后，收录时间可能重置，并需要重新抓取基础资料。</p>
      <p>
        自动维护只处理扫描记录、规划记录和无引用的已完成封面下载记录。完整的一轮结束后等待 24 小时再运行，未完成时 15 分钟后重试。
        留存规则（保留天数）由服务器决定，检查后显示。
      </p>
    </div>

    <p v-if="unknown" role="alert" class="text-xs text-destructive" data-testid="cleanup-unknown">
      无法读取清理状态，当前状态未知。
      <Button size="xs" variant="outline" @click="run.refetch()">重新查询</Button>
    </p>

    <section v-if="current" class="space-y-1.5 border border-[var(--wire-fine)] p-3" aria-live="polite" data-testid="cleanup-run">
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-xs font-medium" data-testid="cleanup-status">{{ STATUS_TEXT[current.status] }}</span>
        <span v-if="active && current.phase" class="text-[11px] text-[var(--text-secondary)]">阶段：{{ LABELS[current.phase] ?? current.phase }}</span>
        <Button v-if="active" class="ml-auto" size="sm" variant="outline" :disabled="current.status === 'stopping' || cancelMutation.isPending.value" data-testid="cleanup-stop" @click="stop">停止</Button>
      </div>
      <p v-if="current.reason" class="text-[11px] text-[var(--text-secondary)]">{{ current.reason }}</p>
      <table class="w-full text-[11px]">
        <thead><tr class="text-left text-[var(--text-secondary)]"><th scope="col" class="font-normal">类别</th><th scope="col" class="font-normal">已检查</th><th scope="col" class="font-normal">已删除</th><th scope="col" class="font-normal">已跳过</th></tr></thead>
        <tbody>
          <tr v-for="id in current.categories" :key="id" :data-testid="`cleanup-count-${id}`">
            <th scope="row" class="font-normal">{{ LABELS[id] }}</th>
            <td class="font-mono">{{ current.counts[id]?.checked ?? 0 }}</td>
            <td class="font-mono">{{ current.counts[id]?.deleted ?? 0 }}</td>
            <td class="font-mono">{{ current.counts[id]?.skipped ?? 0 }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <template v-if="!active">
      <fieldset class="space-y-1">
        <legend class="text-xs font-medium">清理类别</legend>
        <label v-for="c in CATEGORIES" :key="c.id" class="flex min-h-11 items-center gap-2 text-xs rail:min-h-7">
          <input v-model="selected" type="checkbox" :value="c.id" :data-testid="`cleanup-select-${c.id}`" />
          <span class="flex-1">{{ c.label }}</span>
          <span v-if="preview && selected.includes(c.id)" class="font-mono text-[11px]" :data-testid="`cleanup-candidates-${c.id}`">{{ preview.candidates[c.id] ?? 0 }}</span>
        </label>
      </fieldset>

      <ul v-if="rules.length" class="text-[11px] text-[var(--text-secondary)]" data-testid="cleanup-rules">
        <li v-for="rule in rules" :key="rule.id">{{ rule.text }}</li>
      </ul>
      <p v-if="preview && total === 0" class="text-xs" data-testid="cleanup-none">无需清理</p>
      <p v-if="error" role="alert" class="text-xs text-destructive" data-testid="cleanup-error">{{ error }}</p>

      <div class="flex gap-2">
        <Button size="sm" variant="outline" :disabled="checking || selected.length === 0" data-testid="cleanup-check" @click="check">
          {{ checking ? '检查中…' : '检查可清理记录' }}
        </Button>
        <Button size="sm" variant="destructive" :disabled="!preview || checking || total === 0 || startMutation.isPending.value" data-testid="cleanup-start" @click="confirming = true">
          开始清理
        </Button>
      </div>
    </template>

    <Modal :open="confirming" title="确认清理数据库记录" @update:open="confirming = false">
      <ul class="text-xs" data-testid="cleanup-confirm-list">
        <li v-for="c in ordered" :key="c.id" class="flex justify-between gap-3"><span>{{ c.label }}</span><span class="font-mono">{{ preview?.candidates[c.id] ?? 0 }}</span></li>
      </ul>
      <p class="text-xs leading-relaxed">删除的记录无法撤销；实际数量以执行时重新检查为准。</p>
      <div class="flex justify-end gap-2">
        <Button variant="ghost" size="sm" @click="confirming = false">取消</Button>
        <Button variant="destructive" size="sm" data-testid="cleanup-confirm" @click="start">确认清理</Button>
      </div>
    </Modal>
  </div>
</template>
