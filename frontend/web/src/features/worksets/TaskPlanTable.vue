<script setup lang="ts">
import { Badge } from '@/components/ui/badge'
import RowList from '@/features/worksets/RowList.vue'
import type { RenameComponent } from '@/lib/api/types'

/**
 * The frozen proposals of a rename plan: one row per participating member.
 * Review always shows the frozen payload (ADR 0014 §2).
 */
defineProps<{ components: RenameComponent[] }>()

const REASONS: Record<string, string> = {
  METADATA_MISSING: '缺少模板所需的元数据',
  WORK_UNBOUND: '未绑定作品',
  SOURCE_MISSING: '目录不存在',
  SOURCE_NOT_DIRECTORY: '不是目录',
  NAME_TOO_LONG: '名称超过 255 字节',
  NAME_RESERVED: '保留名称',
  WORK_CODE_AMBIGUOUS: '名称无法唯一指向作品',
  TARGET_EXISTS: '目标名称已存在',
  TARGET_COLLISION: '多个文件夹提议同一名称',
  CASE_ONLY_RENAME: '仅大小写不同（Windows 不支持）',
}
const ACTIONS: Record<string, { label: string; tone: 'success' | 'neutral' | 'danger' }> = {
  rename: { label: '重命名', tone: 'success' },
  noop: { label: '无变化', tone: 'neutral' },
  blocked: { label: '阻塞', tone: 'danger' },
}

function name(component: RenameComponent): string {
  return component.action === 'rename' ? `${component.source_name} → ${component.target_name}` : component.source_name
}
</script>

<template>
  <RowList :items="components" :item-key="(component) => component.member_id" testid="task-plan-table">
    <template #default="{ item }">
      <Badge :tone="ACTIONS[item.action]?.tone ?? 'neutral'">{{ ACTIONS[item.action]?.label ?? item.action }}</Badge>
      <span class="truncate font-mono" :title="name(item)">{{ name(item) }}</span>
      <span class="text-[11px] text-[var(--text-secondary)]">{{ item.reason_code ? (REASONS[item.reason_code] ?? item.reason_code) : '' }}</span>
    </template>
  </RowList>
</template>
