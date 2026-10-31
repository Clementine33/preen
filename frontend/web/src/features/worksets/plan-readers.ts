import type {
  AudioOutputSpec,
  ComponentOutcome,
  ConversionPartitionConclusion,
  DesiredProfile,
  FileDecision,
  MemberConclusion as FrozenMemberConclusion,
  OverrideUnit,
  RevisionDetailResponse,
  VariantDecision,
  WorksetMember,
} from '@/lib/api/types'

/**
 * Readers for plan snapshots and desired profiles.
 *
 * Plan components are persisted review data: a snapshot written before the
 * backend's never-null guarantee can still carry `null` where the current
 * contract promises arrays, and opening such a revision must not crash the
 * workbench. Normalizing once on read keeps every downstream render simple.
 */
export function readComponent(component: ComponentOutcome): ComponentOutcome {
  return {
    ...component,
    scope: component.scope ?? '',
    lanes: component.lanes ?? [],
    variant_decisions: (component.variant_decisions ?? []).map(readVariant),
    operations: component.operations ?? [],
    projected_inventory: component.projected_inventory ?? [],
    files: component.files ?? [],
  }
}

export function readVariant(variant: VariantDecision): VariantDecision {
  return { ...variant, decisions: variant.decisions ?? [] }
}

export function componentOperationCount(component: ComponentOutcome): number {
  return (component.operations ?? []).length
}

function decisionsOf(component: ComponentOutcome): FileDecision[] {
  return (component.variant_decisions ?? []).flatMap(variant => variant.decisions ?? [])
}

/**
 * One file's conclusion as a review row. It is read straight from the plan's
 * per-file resolution — no operation is consulted and nothing is re-derived —
 * so every file is named exactly once, with the one thing that happens to it.
 */
export interface DecisionRow {
  resolution: string
  /** The file the decision concludes: an encode's target. */
  name: string
  reasonCode: string
}

/** One folder's rows: every file in it is named exactly once. */
export interface DecisionGroup {
  /** The folder, relative to the member root; empty when the files sit in it. */
  dir: string
  rows: DecisionRow[]
}

/**
 * The component's per-file resolutions grouped by folder. Paths are relative
 * to the member root — the review's header already names the member — so a
 * long CJK name is the only thing a row has to carry.
 */
export function decisionGroups(component: ComponentOutcome, memberRoot = ''): DecisionGroup[] {
  const groups = new Map<string, DecisionRow[]>()
  for (const decision of decisionsOf(component)) {
    const rel = relativeTo(decision.path, memberRoot)
    const row: DecisionRow = {
      resolution: decision.resolution,
      name: posixBase(rel),
      reasonCode: decision.reason_code ?? '',
    }
    const dir = posixDir(rel)
    groups.set(dir, [
      ...(groups.get(dir) ?? []),
      row,
    ])
  }
  return [...groups].map(([dir, rows]) => ({ dir, rows }))
}

/** A frozen path as the member reads it: the member root is the header. */
function relativeTo(path: string, memberRoot: string): string {
  const prefix = memberRoot && !memberRoot.endsWith('/')
    ? `${memberRoot}/`
    : memberRoot
  return prefix && path.startsWith(prefix)
    ? path.slice(prefix.length)
    : path
}

function posixDir(path: string): string {
  const at = path.lastIndexOf('/')
  return at < 0
    ? ''
    : path.slice(0, at)
}

function posixBase(path: string): string {
  const at = path.lastIndexOf('/')
  return at < 0
    ? path
    : path.slice(at + 1)
}

/** Why a kept file was kept, in the plan's own short words. */
const KEEP_REASON_TEXT: Record<string, string> = {
  SOURCE_AMBIGUOUS: '多个同优先级无损源，需区分版本',
  UNMET_TARGET: '目标未满足',
  KEEP_LOSSLESS_TARGET: '已满足无损目标',
  KEEP_ENCODED_SATISFIED: '已满足编码目标',
  // A file of the declared codec whose measured rate cannot judge it and no
  // generation record accepts: kept as it is, since re-encoding a lossy file
  // would not give it the quality it never had. One measured short of the
  // target is UNMET_TARGET instead.
  CONFORMANCE_UNCONFIRMED: '目标符合性未确认',
  // Legacy: what CONFORMANCE_UNCONFIRMED was called before generation records
  // existed. Kept so revisions planned by an older build still read in words.
  QUALITY_UNKNOWN: '质量未知',
}

export function keepReasonText(reasonCode: string | undefined): string {
  if (!reasonCode)
    return '原样保留'
  return KEEP_REASON_TEXT[reasonCode] ?? reasonCode
}

/** One decision's own short word: what happens to the file. */
const RESOLUTION_TEXT: Record<string, string> = {
  keep: '保留',
  delete: '删除',
  encode: '生成',
}

export function resolutionText(resolution: string): string {
  return RESOLUTION_TEXT[resolution] ?? resolution
}

/** One partition's independent facts within a member's planned components. */
export interface MemberFacts {
  matched: ConversionPartitionConclusion
  unmatched: ConversionPartitionConclusion
}

export const PARTITION_TEXT: Record<keyof MemberFacts, string> = {
  matched: '无音效',
  unmatched: '有音效',
}

export interface MemberConclusion {
  tone: 'neutral' | 'success' | 'warning' | 'danger'
  /** The row's main status; conversion scope belongs in the detail. */
  label: string
  /** The filter matching the main status shown in the badge. */
  filter: 'change' | 'warn' | 'blocked' | 'source_conflict' | null
  /** Both partitions' own facts, so neither side's state hides behind a word. */
  detail: string
}

/** Wording only: status and independent partition facts come from the server. */
export function memberConclusion(input: FrozenMemberConclusion): MemberConclusion {
  if (input.excluded) {
    return {
      tone: 'neutral',
      label: '已排除',
      detail: '本操作已排除该文件夹。',
      filter: null,
    }
  }
  if (input.input === 'none') {
    return {
      tone: 'neutral',
      label: '未参与',
      detail: '该文件夹在此版本中没有规划输入。',
      filter: null,
    }
  }
  if (input.input === 'missing') {
    return {
      tone: 'danger',
      label: '输入缺失',
      detail: '规划时未在扫描结果中找到该文件夹。',
      filter: 'blocked',
    }
  }
  const task = input.task
  if (!task) {
    return {
      tone: 'neutral',
      label: '未参与',
      detail: '该文件夹在此版本中没有规划输入。',
      filter: null,
    }
  }
  const detail = detailText(task)
  switch (task.status) {
    case 'blocked': return {
      tone: 'danger',
      label: '阻塞',
      detail,
      filter: 'blocked',
    }
    case 'source_ambiguous': return {
      tone: 'danger',
      label: '源竞争',
      detail: `存在多个同优先级无损源，无法唯一选源。${detail}`,
      filter: 'source_conflict',
    }
    case 'partially_unmet': return {
      tone: 'warning',
      label: '部分未满足',
      detail,
      filter: 'warn',
    }
    case 'unmet': return {
      tone: 'warning',
      label: '未满足',
      detail,
      filter: 'warn',
    }
    case 'changed': return {
      tone: 'success',
      label: '有变化',
      detail,
      filter: 'change',
    }
    case 'not_applicable': return {
      tone: 'neutral',
      label: '无适用',
      detail,
      filter: null,
    }
    case 'unchanged': return {
      tone: 'neutral',
      label: '无需转换',
      detail,
      filter: null,
    }
  }
}

/** One line per partition: what it does, or why it does nothing. */
function detailText(facts: MemberFacts): string {
  return (['matched', 'unmatched'] as const)
    .map(key => `${PARTITION_TEXT[key]}：${sideText(facts[key])}`)
    .join('；')
}

function sideText(side: ConversionPartitionConclusion): string {
  if (side.blocked)
    return '阻塞'
  if (side.unmet)
    return side.changes ? '目标未满足，其余按可用源转换' : '目标未满足，现有文件按可用源保留'
  if (side.changes)
    return '将转换'
  return side.applicable ? '无需改动' : '无适用文件'
}

/**
 * Deep-copies a desired profile into plain objects. Vue props are reactive
 * proxies, which structuredClone refuses; the profile is plain JSON data, so
 * an explicit copy is both safe and clearer about what is carried over.
 */
export function cloneProfile(value: unknown): DesiredProfile {
  const src = (value ?? {}) as DesiredProfile
  const out: DesiredProfile = {}
  if (src.lossless) {
    out.lossless = {
      codec: src.lossless.codec,
      ...(src.lossless.quality
        ? {
            quality: { ...src.lossless.quality },
          }
        : {}),
    }
  }
  if (src.encoded) {
    out.encoded = {
      codec: src.encoded.codec,
      ...(src.encoded.quality
        ? {
            quality: { ...src.encoded.quality },
          }
        : {}),
    }
  }
  return out
}

/** One lane of a profile: "WAV", "MP3 320", or nothing when the lane is absent. */
function laneText(spec: AudioOutputSpec | undefined): string | null {
  if (!spec?.codec)
    return null
  const codec = spec.codec.toUpperCase()
  const bitrate = spec.quality?.bitrate
  return bitrate
    ? `${codec} ${bitrate}`
    : codec
}

/** A desired profile in one line: the exact output set the revision asks for. */
export function profileText(profile: DesiredProfile | undefined): string {
  if (!profile)
    return '未设置'
  const parts = [
    laneText(profile.lossless),
    laneText(profile.encoded),
  ].filter(
    (part): part is string => part !== null,
  )
  // A profile that declares nothing is a declaration, not a gap: the partition
  // keeps no managed audio.
  return parts.length > 0
    ? parts.join(' + ')
    : '不需要'
}

const MODE_TEXT: Record<string, string> = { strict: '严格', available_sources: '可用源' }

/** The label of a conversion mode, the raw value when unknown. */
export const modeText = (mode: string): string => MODE_TEXT[mode] ?? mode

/**
 * One unit's value as the editors show it — the same rendering for a stored
 * value, a common value and a pending one, so "独立值" never has to stand in
 * for the value itself.
 */
export function unitValueText(unit: OverrideUnit, value: unknown): string {
  switch (unit) {
    case 'mode': {
      const mode = typeof value === 'string'
        ? value
        : ''
      return MODE_TEXT[mode] ?? (mode || '未设置')
    }
    case 'no_effect_tags':
    case 'other_tags': {
      const tags = (value as string[] | undefined) ?? []
      return tags.length > 0
        ? tags.join(', ')
        : '（空）'
    }
    default:
      return profileText(value as DesiredProfile | undefined)
  }
}

/**
 * The conclusion of every member of a record, keyed by member id. The list asks
 * for each member several times (rows, status filters) and a full-library
 * record holds thousands of members and components, so the server conclusions are indexed once
 * here by member identity. A record without a plan has no entries.
 */
export function memberConclusions(
  view: RevisionDetailResponse | null | undefined,
  members: WorksetMember[],
): Map<string, MemberConclusion> {
  const out = new Map<string, MemberConclusion>()
  if (!view)
    return out
  const frozen = new Map(view.members.map(m => [m.member_id, m]))
  for (const member of members) {
    const conclusion = frozen.get(member.member_id)
    if (conclusion)
      out.set(member.member_id, memberConclusion(conclusion))
  }
  return out
}
