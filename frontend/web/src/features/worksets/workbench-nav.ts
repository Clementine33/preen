import type { RouteLocationRaw } from 'vue-router'
import type { Operation } from '@/lib/api/types'

/**
 * The workbench navigation definition.
 *
 * The inline sidebar of the mid/wide tiers and the narrow drawer both render
 * this one list, exactly as the global rail and bottom bar share
 * `global-nav` — two renderers, never two hard-coded entry lists.
 * Entries carry only id/label/to/children, and every location is built
 * from a *named* route plus `libraryId`, so a path rename cannot quietly
 * break navigation.
 */
export type WorkbenchNavId = 'overview' | 'conversion' | 'conversion-settings' | 'rename' | 'metadata-refresh'

export interface WorkbenchNavItem {
  id: WorkbenchNavId
  label: string
  to: RouteLocationRaw
  children?: WorkbenchNavItem[]
}

/** The workbench's own entries for one workset: one section per operation,
 *  and the settings page nested under 转换 (the only nesting defined). */
export function workbenchNav(libraryId: string): WorkbenchNavItem[] {
  return [
    {
      id: 'overview',
      label: '概览与成员',
      to: { name: 'workbench-overview', params: { libraryId } },
    },
    {
      id: 'conversion',
      label: '转换',
      to: { name: 'conversion', params: { libraryId } },
      children: [
        {
          id: 'conversion-settings',
          label: '转换全局设置',
          to: { name: 'conversion-settings', params: { libraryId } },
        },
      ],
    },
    {
      id: 'rename',
      label: '重命名',
      to: { name: 'rename', params: { libraryId } },
    },
    {
      id: 'metadata-refresh',
      label: '获取元数据',
      to: { name: 'metadata-refresh', params: { libraryId } },
    },
  ]
}

/** Route name → position in the workbench navigation. */
export type WorkbenchRouteName =
  | 'workbench-overview'
  | 'overview-files'
  | 'conversion'
  | 'conversion-member-files'
  | 'conversion-settings'
  | 'conversion-member'
  | 'conversion-member-edit'
  | 'conversion-batch-edit'
  | 'conversion-execution'
  | 'rename'
  | 'metadata-refresh'

export interface WorkbenchNavPosition {
  /** The entry that IS the current page. */
  current: WorkbenchNavId
  /**
   * The entry whose section contains the current page. It is shown as the
   * owning level, never as a second current page.
   */
  parent?: WorkbenchNavId
}

const POSITION_BY_ROUTE_NAME: Record<WorkbenchRouteName, WorkbenchNavPosition> = {
  'workbench-overview': { current: 'overview' },
  'overview-files': { current: 'overview' },
  conversion: { current: 'conversion' },
  'conversion-member-files': { current: 'conversion' },
  'conversion-settings': { current: 'conversion-settings', parent: 'conversion' },
  // The detail, batch and execution entries are carriers of the conversion
  // list, so the list stays the current entry and 转换全局设置 is never faked
  // as current.
  'conversion-member': { current: 'conversion' },
  'conversion-member-edit': { current: 'conversion' },
  'conversion-batch-edit': { current: 'conversion' },
  'conversion-execution': { current: 'conversion' },
  rename: { current: 'rename' },
  'metadata-refresh': { current: 'metadata-refresh' },
}

export function workbenchNavPosition(routeName: unknown): WorkbenchNavPosition | null {
  return typeof routeName === 'string' && routeName in POSITION_BY_ROUTE_NAME
    ? POSITION_BY_ROUTE_NAME[routeName as WorkbenchRouteName]
    : null
}

/**
 * Why 转换全局设置 cannot be edited right now: generating, or orphaned
 * against a deleted library. Shared so the overview and the conversion page
 * disable the entry with the same words, never one reason each.
 */
export function settingsEditBlockedReason(operation: Operation | null | undefined): string | null {
  if (operation?.planning_state === 'orphaned') return '媒体库已删除：该记录只读'
  if (operation?.active_generation) return '正在生成计划版本：完成后才能修改设置'
  return null
}
