import type { MemberStatus } from '@/lib/api/types'

type Tone = 'neutral' | 'brand' | 'success' | 'warning' | 'danger'

/** A member's own state, in the words both the exception list and the member page use. */
export const MEMBER_STATUS: Record<MemberStatus, { label: string, tone: Tone }> = {
  pending: { label: '未执行', tone: 'neutral' },
  running: { label: '执行中', tone: 'brand' },
  succeeded: { label: '已完成', tone: 'success' },
  failed: { label: '失败', tone: 'danger' },
  canceled: { label: '已取消', tone: 'warning' },
}

/** Count order on the session page: outcome first, what never ran last. */
export const MEMBER_COUNT_ORDER: MemberStatus[] = [
  'succeeded',
  'failed',
  'canceled',
  'running',
  'pending',
]
