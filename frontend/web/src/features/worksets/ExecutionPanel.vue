<script setup lang="ts">
import type { ExecutionStatus, ExecutionView, WorksetMember } from '@/lib/api/types'
import { Folder } from '@lucide/vue'
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { Badge } from '@/components/ui/badge'
import { MEMBER_COUNT_ORDER, MEMBER_STATUS } from '@/features/worksets/execution-labels'
import { sessionFailureText } from '@/features/worksets/session-failure'

/**
 * Live progress and the observed result of one execution session, read as
 * Member Results (the `view=members` shape): the session's progress, how many
 * members ended in each state, and only the exceptions — failed, canceled,
 * running or with an inventory warning — capped at what the server sent. A
 * member's own components and files live on its own page, linked from here, so
 * this panel's size does not depend on how many members ran.
 */
const props = defineProps<{
  view: ExecutionView
  /** The record's members, to name a folder the way the user chose it. */
  members?: WorksetMember[]
  /** Builds the member execution page's link; absent renders plain text. */
  memberRoute?: (memberId: string) => object
}>()

const STATUS: Record<ExecutionStatus, { label: string, tone: 'neutral' | 'brand' | 'success' | 'warning' | 'danger' }> = {
  queued: { label: '排队中', tone: 'neutral' },
  running: { label: '执行中', tone: 'brand' },
  succeeded: { label: '已完成', tone: 'success' },
  failed: { label: '失败', tone: 'danger' },
  canceled: { label: '已取消', tone: 'warning' },
  interrupted: { label: '已中断', tone: 'danger' },
}

const status = computed(() => STATUS[props.view.status])
const running = computed(() => props.view.status === 'running' || props.view.status === 'queued')

function shortPath(path: string): string {
  const parts = path.split('/')
  return parts.length <= 2
    ? path
    : `…/${parts.slice(-2).join('/')}`
}

const names = computed(() => new Map((props.members ?? []).map(member => [member.member_id, member.folder_name])))
function memberName(memberId: string, rootPath: string): string {
  return names.value.get(memberId) ?? shortPath(rootPath)
}

const memberBlock = computed(() => props.view.members ?? null)
const hiddenAttention = computed(() => Math.max(0, (memberBlock.value?.attention_total ?? 0) - (memberBlock.value?.attention.length ?? 0)))

/**
 * What this session covered, when it was scoped. A revision still runs once, so
 * a scoped run spends it: the folders it left out are not pending here, they are
 * the next plan's work, and the panel has to say so rather than let the reader
 * find out from a missing card.
 */
const scope = computed(() => {
  const selected = props.view.selected_member_ids ?? []
  const total = props.members?.length ?? 0
  if (selected.length === 0 || total === 0 || selected.length >= total)
    return null
  return { selected: selected.length, total }
})
</script>

<template>
  <section aria-label="执行结果" class="px-3 py-2" data-testid="execution-panel">
    <div class="flex flex-wrap items-center gap-2 border-b border-[var(--wire-strong)] px-1 py-2.5">
      <Badge :tone="status.tone" size="md" data-testid="execution-status">{{ status.label }}</Badge>
      <span class="font-mono text-[11px]" data-testid="execution-progress">
        组件 {{ view.completed_components }}/{{ view.total_components }} · 操作 {{ view.completed_operations }}/{{ view.total_operations }}
      </span>
      <Badge tone="neutral">{{ view.options?.delete_mode === 'hard' ? '硬删除' : '软删除' }}</Badge>
      <span v-if="running && view.current_root" class="font-mono text-[10px] text-[var(--text-secondary)]" :title="view.current_root">
        {{ shortPath(view.current_root) }}
        <template v-if="view.current_component_id"> · {{ view.current_component_id }}</template>
      </span>
    </div>

    <p v-if="view.error_code" class="mt-1 text-[11px] text-[var(--danger-ink)]" role="alert" data-testid="execution-error">
      执行失败：{{ sessionFailureText(view.error_code, view.error_message) }}
    </p>
    <p v-if="view.status === 'interrupted'" class="mt-1 text-[11px] text-[var(--danger-ink)]">
      执行被后端中断（进程重启）：已完成的组件结果保留，未执行的操作保持磁盘原样。需重新生成并确认新版本才能继续。
    </p>
    <p v-else-if="view.status === 'canceled'" class="mt-1 text-[11px] text-[var(--warning-ink)]">
      执行已取消：已完成的组件结果保留，未执行的操作保持磁盘原样。
    </p>

    <p
      v-if="scope"
      class="mt-2 border border-[var(--wire-mid)] bg-[var(--warning-weak,var(--muted))] px-3 py-2 text-[11px]"
      data-testid="execution-scope"
      role="status"
    >
      本次只执行了选中的 {{ scope.selected }} 个文件夹（记录共 {{ scope.total }} 个）；其余文件夹需重新生成计划后执行。
    </p>

    <template v-if="memberBlock">
      <ul class="mt-3 flex flex-wrap gap-1.5" aria-label="成员执行状态" data-testid="execution-member-counts">
        <li v-for="key in MEMBER_COUNT_ORDER" :key="key" class="flex items-center gap-1">
          <Badge :tone="MEMBER_STATUS[key].tone">{{ MEMBER_STATUS[key].label }}</Badge>
          <span class="font-mono text-[11px] tabular-nums" :data-testid="`execution-count-${key}`">{{ memberBlock.counts[key] }}</span>
        </li>
      </ul>

      <ul v-if="memberBlock.attention.length > 0" class="mt-3 border border-[var(--wire-mid)]" data-testid="execution-attention">
        <li
          v-for="member in memberBlock.attention"
          :key="member.member_id"
          class="border-b border-border/60 px-3 py-2.5 last:border-b-0"
          data-testid="execution-attention-item"
          :data-member-status="member.status"
        >
          <div class="flex items-start gap-2.5">
            <Folder class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            <component
              :is="memberRoute ? RouterLink : 'span'"
              :to="memberRoute?.(member.member_id)"
              class="min-w-0 flex-1 line-clamp-2 break-words font-heading text-xs font-semibold leading-relaxed"
              :class="memberRoute ? 'hover:underline' : ''"
              :title="member.root_path"
              data-testid="execution-member-name"
            >
              {{ memberName(member.member_id, member.root_path) }}
            </component>
            <Badge :tone="MEMBER_STATUS[member.status].tone" class="shrink-0" data-testid="execution-member-status">
              {{ MEMBER_STATUS[member.status].label }}
            </Badge>
          </div>
          <div class="mt-1 pl-6.5 text-[11px] tabular-nums text-muted-foreground" data-testid="execution-member-counts">
            组件 {{ member.completed_components }}/{{ member.total_components }} · 操作
            {{ member.completed_operations }}/{{ member.total_operations }}
          </div>
          <p v-if="member.error_message" class="mt-1 pl-6.5 text-[11px] text-[var(--danger-ink)]">{{ member.error_message }}</p>
          <p v-if="member.inventory_sync_error" class="mt-1 pl-6.5 text-[10px] text-[var(--warning-ink)]" data-testid="execution-inventory-warning">
            磁盘结果与媒体库索引未完全同步：{{ member.inventory_sync_error }}
          </p>
        </li>
      </ul>
      <p v-if="hiddenAttention > 0" class="mt-2 px-1 text-[11px] text-[var(--text-secondary)]" data-testid="execution-attention-more">
        另有 {{ hiddenAttention }} 个
      </p>
    </template>
  </section>
</template>
