<script setup lang="ts">
import { computed } from 'vue'
import { Badge } from '@/components/ui/badge'
import RowList from '@/features/worksets/RowList.vue'
import type { RenameComponent, WorksetMember } from '@/lib/api/types'

/**
 * The frozen proposals of a rename plan, as a static two-line table: the old
 * name above, the new name below, so the eye zigzags old → new down the list.
 * A whole-library plan is mostly folders whose name does not change, so those
 * are only counted; only renames, skipped (blocked) folders and followed hand
 * renames are drawn. Both lines carry the parent folder in grey: a rename only
 * changes the last segment, and the shared prefix shows it stays in place.
 * Review always shows the frozen payload (ADR 0014 §2).
 */
const props = defineProps<{ components: RenameComponent[]; members?: WorksetMember[] }>()

const REASONS: Record<string, string> = {
  METADATA_MISSING: '缺少模板所需的元数据',
  WORK_UNBOUND: '未绑定作品',
  SOURCE_MISSING: '目录不存在',
  SOURCE_NOT_DIRECTORY: '不是目录',
  NAME_TOO_LONG: '名称超过 255 字节',
  NAME_RESERVED: '保留名称',
  NAME_CODE_INVALID: '名称须恰好包含一个作品号',
  WORK_CODE_AMBIGUOUS: '名称无法唯一指向作品',
  TARGET_EXISTS: '目标名称已存在',
  TARGET_COLLISION: '多个文件夹提议同一名称',
  CASE_ONLY_RENAME: '仅大小写不同（Windows 不支持）',
}
const ACTIONS: Record<string, { label: string; tone: 'success' | 'neutral' | 'warning' }> = {
  rename: { label: '重命名', tone: 'success' },
  noop: { label: '跟随', tone: 'neutral' },
  blocked: { label: '跳过', tone: 'warning' },
}

const shown = computed(() => props.components.filter((c) => c.action !== 'noop' || c.observed_name))
const counts = computed(() => {
  let rename = 0
  let skipped = 0
  for (const c of props.components) {
    if (c.action === 'rename') rename++
    else if (c.action === 'blocked') skipped++
  }
  return { rename, skipped, unchanged: props.components.length - shown.value.length }
})

// The parent folder tells apart two folders that share a name; one lookup per
// member, built once, rather than a scan per row.
const parents = computed(() => {
  const map = new Map<string, string>()
  for (const member of props.members ?? []) {
    const slash = member.rel_path.lastIndexOf('/')
    if (slash > 0) map.set(member.member_id, member.rel_path.slice(0, slash + 1))
  }
  return map
})

function secondLine(component: RenameComponent): string {
  if (component.action === 'blocked') return REASONS[component.reason_code ?? ''] ?? component.reason_code ?? ''
  if (component.action === 'noop') return '名称已是目标名称，仅更新记录'
  return component.target_name
}
</script>

<template>
  <div>
    <p class="text-[11px] text-[var(--text-secondary)]" data-testid="plan-summary">
      重命名 {{ counts.rename }} · 跳过 {{ counts.skipped }} · {{ counts.unchanged }} 个名称不变（未显示）
    </p>
    <RowList
      :items="shown"
      :item-key="(component) => component.member_id"
      testid="task-plan-table"
      row-class="h-12 [contain-intrinsic-size:auto_48px]"
    >
      <template #default="{ item }">
        <Badge :tone="ACTIONS[item.action]?.tone ?? 'neutral'">{{ ACTIONS[item.action]?.label ?? item.action }}</Badge>
        <span class="flex min-w-0 flex-col">
          <span class="truncate font-mono" :title="item.source_name">
            <span v-if="parents.get(item.member_id)" class="text-[var(--text-muted)]">{{ parents.get(item.member_id) }}</span>{{ item.source_name }}
            <span v-if="item.observed_name" class="font-sans text-[11px] text-[var(--text-secondary)]">
              已手动改名为 {{ item.observed_name }}
            </span>
          </span>
          <span class="truncate font-mono text-[var(--text-secondary)]" :title="secondLine(item)">
            ↘ <span v-if="item.action !== 'blocked' && parents.get(item.member_id)" class="text-[var(--text-muted)]">{{ parents.get(item.member_id) }}</span>{{ secondLine(item) }}
          </span>
        </span>
        <span class="text-[11px] text-[var(--text-secondary)]">
          <template v-if="item.rebind_to">RJ 变更 {{ item.work_code }} → {{ item.rebind_to }}</template>
          <template v-else-if="item.custom_name">自定义</template>
        </span>
      </template>
    </RowList>
  </div>
</template>
