import type { ConversionMemberConclusion, MemberConclusion } from '@/lib/api/types'

/** A frozen server response; tests choose facts without recalculating plan rules. */
export function frozenConclusion(
  status: ConversionMemberConclusion['status'] = 'unchanged',
  overrides: Partial<MemberConclusion> = {},
): MemberConclusion {
  return {
    member_id: 'm',
    excluded: false,
    input: 'valid',
    has_components: true,
    task: {
      status,
      matched: {
        applicable: true,
        blocked: false,
        unmet: false,
        changes: false,
      },
      unmatched: {
        applicable: false,
        blocked: false,
        unmet: false,
        changes: false,
      },
    },
    ...overrides,
  }
}
