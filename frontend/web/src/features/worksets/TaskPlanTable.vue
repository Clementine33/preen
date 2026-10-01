<script setup lang="ts">
import { computed } from 'vue'
import { Badge } from '@/components/ui/badge'
import type { MetadataComponent, RenameComponent, WorksetMember } from '@/lib/api/types'

/**
 * The frozen proposals of a rename or metadata refresh plan: one row per
 * participating member. Review always shows the frozen payload (ADR 0014 §2).
 */
const props = defineProps<{
  components: (RenameComponent | MetadataComponent)[]
  members: WorksetMember[]
}>()

const REASONS: Record<string, string> = {
  METADATA_MISSING: '缺少模板所需的元数据',
  WORK_UNBOUND: '未绑定作品',
  NO_WORK_BINDING: '未绑定作品，跳过',
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
  refresh: { label: '刷新', tone: 'success' },
  noop: { label: '无变化', tone: 'neutral' },
  skip: { label: '跳过', tone: 'neutral' },
  blocked: { label: '阻塞', tone: 'danger' },
}

const folderName = computed(() => new Map(props.members.map((member) => [member.member_id, member.folder_name])))

function reason(code?: string): string {
  return code ? (REASONS[code] ?? code) : ''
}
</script>

<template>
  <table class="w-full text-left text-xs" data-testid="task-plan-table">
    <tbody>
      <tr v-for="component in components" :key="component.member_id" class="border-b border-border align-top">
        <td class="py-1.5 pr-2">
          <Badge :tone="ACTIONS[component.action]?.tone ?? 'neutral'">
            {{ ACTIONS[component.action]?.label ?? component.action }}
          </Badge>
        </td>
        <td class="min-w-0 py-1.5 pr-2 font-mono break-all">
          <template v-if="'target_name' in component">
            {{ component.source_name }}
            <template v-if="component.action === 'rename'"> → <strong>{{ component.target_name }}</strong></template>
          </template>
          <template v-else>
            {{ folderName.get(component.member_id) ?? component.target.rel_path }}
            <span v-if="component.target.rj" class="text-[var(--text-muted)]"> · {{ component.target.rj }}</span>
          </template>
        </td>
        <td class="py-1.5 text-[11px] text-[var(--text-secondary)]">{{ reason(component.reason_code) }}</td>
      </tr>
    </tbody>
  </table>
</template>
