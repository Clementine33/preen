import { Layers, ListMusic, Settings2, SquareLibrary } from '@lucide/vue'
import type { Component } from 'vue'
import type { RouteLocationRaw } from 'vue-router'

/**
 * The global navigation definition.
 *
 * One list, two renderers: the desktop rail and the mobile bottom bar both
 * iterate this array, so their labels, order and targets cannot drift apart.
 * Entries carry only id/label/icon/to — no permissions, no business state, no
 * plugin registry. The media-library *list* is page content on /worksets,
 * never a global entry (ADR 0001). 设置 is a global entry too, but apart from
 * the content entries: the rail sets it at its foot, the phone menu last.
 */
export type GlobalNavId = 'works' | 'collections' | 'worksets' | 'settings'

export interface GlobalNavItem {
  id: GlobalNavId
  label: string
  icon: Component
  to: RouteLocationRaw
}

export const GLOBAL_NAV: readonly GlobalNavItem[] = [
  { id: 'works', label: '作品库', icon: SquareLibrary, to: '/works' },
  { id: 'collections', label: '合集', icon: Layers, to: '/collections' },
  { id: 'worksets', label: '工作集', icon: ListMusic, to: '/worksets' },
]

export const SETTINGS_NAV: GlobalNavItem = { id: 'settings', label: '设置', icon: Settings2, to: '/settings' }

/**
 * Route name → owning global entry. The current item is derived from the
 * router's own route name, never from a stored `activeNavId` that could drift
 * from the URL. An explicit mapping — not a URL-prefix guess — keeps every
 * workbench child route under 工作集 without pretending to be a second route
 * table: the test asserts it covers exactly the application router's names.
 */
const NAV_OWNER_BY_ROUTE_NAME = {
  works: 'works',
  'work-detail': 'works',
  collections: 'collections',
  'collection-detail': 'collections',
  worksets: 'worksets',
  'workbench-overview': 'worksets',
  'overview-files': 'worksets',
  conversion: 'worksets',
  'conversion-member-files': 'worksets',
  'conversion-settings': 'worksets',
  'conversion-member': 'worksets',
  'conversion-member-edit': 'worksets',
  'conversion-batch-edit': 'worksets',
  'conversion-execution': 'worksets',
  rename: 'worksets',
  'metadata-refresh': 'worksets',
  settings: 'settings',
  // An address that names no page of the workbench is still reached inside the
  // workbench: the entry that offers the way back is 工作集.
  'not-found': 'worksets',
} as const satisfies Record<string, GlobalNavId>

export type AppRouteName = keyof typeof NAV_OWNER_BY_ROUTE_NAME

export function globalNavOwner(routeName: unknown): GlobalNavId | null {
  return typeof routeName === 'string' && routeName in NAV_OWNER_BY_ROUTE_NAME
    ? NAV_OWNER_BY_ROUTE_NAME[routeName as AppRouteName]
    : null
}
