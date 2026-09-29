import type { RouteLocationNormalized, Router } from 'vue-router'

/**
 * The URL parameters each page supports (ADR 0003 §5).
 *
 * Addresses carry identities, never names: a page's own parameters are the few
 * closed vocabularies below, and everything else — a retired `folder`, a search
 * term, a campaign tag someone pasted on — is removed at the route entry. One
 * table therefore answers what any address can mean.
 */
export const MEMBER_FILTERS = ['all', 'change', 'warn', 'blocked', 'excluded'] as const

/** The conclusion filter of the conversion list. */
export type MemberFilter = (typeof MEMBER_FILTERS)[number]

/** The values each parameter may take. A parameter outside this table is refused. */
const ALLOWED_VALUES: Record<string, readonly string[]> = {
  view: ['current', 'plan'],
  filter: MEMBER_FILTERS,
}

/**
 * Parameters whose values are identities rather than a closed vocabulary: each
 * carries its own shape instead of an enumerated list.
 */
const OPEN_VALUE: Record<string, RegExp> = {
  // The selected work of the wall: a work number in either spelling the API
  // accepts (6 or 8 digits, any case); 7 digits is no work number (docs/api.md).
  rj: /^RJ(\d{6}|\d{8})$/i,
}

/**
 * The canonical spelling of a work number — `RJ` plus 8 digits — or null when
 * the text is none. `RJ123456` and `rj00123456` are the same work, and the
 * listing only ever answers the canonical form, so an address is compared in it.
 */
export function canonicalRj(raw: string): string | null {
  const match = /^RJ(\d{6}|\d{8})$/i.exec(raw.trim())
  if (!match) return null
  return `RJ${match[1].padStart(8, '0')}`
}

/**
 * The parameters each named route keeps. A route absent here keeps none.
 *
 * The conversion list's filter travels with its whole subtree: the carriers are
 * the same list with a detail open, so the list behind them keeps its view.
 */
const CONVERSION_SUBTREE = [
  'conversion',
  'conversion-member',
  'conversion-member-edit',
  'conversion-settings',
  'conversion-execution',
  'conversion-batch-edit',
]

const ALLOWED_PARAMS: Record<string, readonly string[]> = {
  ...Object.fromEntries(CONVERSION_SUBTREE.map((name) => [name, ['filter']])),
  'conversion-member-files': ['filter', 'view'],
  works: ['rj'],
}

/** The filter an address asks for, or the unfiltered list. */
export function readMemberFilter(value: unknown): MemberFilter {
  return MEMBER_FILTERS.find((candidate) => candidate === value) ?? 'all'
}

/**
 * Address hygiene: a page keeps only the parameters it declares, with values
 * from its closed vocabulary. The rewrite replaces the entry instead of pushing
 * a new one, and it happens only when something is actually removed — so it
 * neither creates history nor redirects in a loop.
 */
export function installAddressHygiene(router: Router): void {
  router.beforeEach((to) => {
    const query = cleanedQuery(to)
    if (query === null) return true
    return { path: to.path, query, hash: '', replace: true }
  })
}

/**
 * What the rules read of an address: a resolved location and a normalized one
 * both qualify, which is what lets the rules be asserted on a plain resolve.
 */
export type AddressLike = Pick<RouteLocationNormalized, 'query' | 'hash'> & { name?: unknown }

/**
 * The query one route may keep: its own parameters, with values from the closed
 * vocabulary, and nothing else. It answers null when the address is already
 * clean, so the guard neither redirects nor writes history for nothing.
 */
export function cleanedQuery(route: AddressLike): Record<string, string> | null {
  const allowed = typeof route.name === 'string' ? (ALLOWED_PARAMS[route.name] ?? []) : []
  const kept: Record<string, string> = {}
  for (const key of allowed) {
    const value = route.query[key]
    if (typeof value === 'string' && value !== '') {
      const closed = ALLOWED_VALUES[key]
      const open = OPEN_VALUE[key]
      if (closed?.includes(value) ?? (open?.test(value) ?? false)) {
        kept[key] = value
      }
    }
  }
  const carried = Object.keys(route.query)
  const clean = carried.length === Object.keys(kept).length && carried.every((key) => key in kept)
  if (clean && !route.hash) return null
  return kept
}
