import type {
  DraftMember,
  OperationDraftDocument,
  OverrideSet,
  OverrideUnit,
} from '@/lib/api/types'

/**
 * Editing intents over the sparse draft.
 *
 * Every edit names exactly one target and one intent per unit. The document is
 * rebuilt from the persisted base document plus those intents — the form never
 * becomes the saved document, so an untouched unit is never turned into a
 * member override and an explicit "inherit" really deletes the override.
 */

export type EditTarget =
  | { kind: 'batch'; memberIds: string[] }
  | { kind: 'member'; memberId: string }

/** keep = leave the stored state untouched; set = write an explicit value. */
export type UnitIntent<T> = { intent: 'keep' } | { intent: 'set'; value: T } | { intent: 'inherit' }

export type ParticipationIntent = 'keep' | 'participate' | 'exclude'

export interface EditIntent {
  units: Partial<Record<OverrideUnit, UnitIntent<unknown>>>
  participation: ParticipationIntent
}

export const EMPTY_INTENT: EditIntent = { units: {}, participation: 'keep' }

/** Where a unit's current value comes from for the edited target. */
export type UnitSource = 'common' | 'member' | 'mixed'

export interface UnitValue {
  /**
   * Defined when every member's EFFECTIVE value agrees — including the case
   * where some members reach that value through an explicit override. It is
   * undefined only when the target genuinely holds several values, which the
   * form shows as "多种值" and can never save as one business value.
   */
  value: unknown
  /** `common` = nobody overrides, `member` = everybody does, else `mixed`. */
  source: UnitSource
  /** How many members of the target explicitly override the unit. */
  overridden: number
  members: number
}

export const OVERRIDE_UNITS: OverrideUnit[] = ['mode', 'no_effect_tags', 'other_tags', 'matched', 'unmatched', 'other']

function commonValue(doc: OperationDraftDocument, unit: OverrideUnit): unknown {
  switch (unit) {
    case 'mode':
      return doc.mode ?? 'strict'
    case 'no_effect_tags':
    case 'other_tags':
      return doc[unit] ?? []
    case 'matched':
      return doc.matched
    case 'other':
      return doc.other ?? doc.unmatched
    case 'unmatched':
      return doc.unmatched
  }
}

function memberRecord(doc: OperationDraftDocument, memberId: string): DraftMember | undefined {
  return doc.members.find((m) => m.member_id === memberId)
}

function overrideValue(record: DraftMember | undefined, unit: OverrideUnit): { present: boolean; value: unknown } {
  const overrides = record?.overrides
  if (!overrides) return { present: false, value: undefined }
  switch (unit) {
    case 'mode':
      return overrides.mode === undefined
        ? { present: false, value: undefined }
        : { present: true, value: overrides.mode }
    case 'no_effect_tags':
    case 'other_tags':
      return overrides[unit] === undefined
        ? { present: false, value: undefined }
        : { present: true, value: overrides[unit] }
    case 'matched':
      return overrides.matched === undefined
        ? { present: false, value: undefined }
        : { present: true, value: overrides.matched }
    case 'other':
      return overrides.other === undefined
        ? { present: false, value: undefined }
        : { present: true, value: overrides.other }
    case 'unmatched':
      return overrides.unmatched === undefined
        ? { present: false, value: undefined }
        : { present: true, value: overrides.unmatched }
  }
}

/** Structural equality of two unit values (a mode string, tags, a profile). */
export function sameUnitValue(a: unknown, b: unknown): boolean {
  return canonical(a) === canonical(b)
}

/** JSON with sorted keys and no undefined, so key order never reads as a change. */
function canonical(value: unknown): string {
  return JSON.stringify(value, (_key, v) =>
    v && typeof v === 'object' && !Array.isArray(v)
      ? Object.fromEntries(Object.entries(v).sort(([x], [y]) => (x < y ? -1 : x > y ? 1 : 0)))
      : v,
  )
}

/**
 * Whether two draft documents hold the same persisted content. Both sides come
 * from the server's canonical JSON (or from a save of it), so an exact string
 * compare is enough — and a content that merely was NOT changed by an unrelated
 * event (a generation publication advancing the version) compares equal.
 */
export function sameDocument(a: OperationDraftDocument, b: OperationDraftDocument): boolean {
  return sameUnitValue(withDefaults(a), withDefaults(b))
}

/** The server omits empty mode/delete_mode: absent and the default are one fact. */
function withDefaults(doc: OperationDraftDocument): OperationDraftDocument {
  return { ...doc, other_tags: doc.other_tags ?? [], mode: doc.mode ?? 'available_sources', delete_mode: doc.delete_mode ?? 'soft' }
}

/**
 * Reads the current value and provenance of one unit for a target. A batch
 * whose members disagree reports `mixed`: a mixed value is a display state,
 * never something that can be saved.
 */
export function readUnit(doc: OperationDraftDocument, target: EditTarget, unit: OverrideUnit): UnitValue {
  const memberIds = target.kind === 'member' ? [target.memberId] : target.memberIds
  const common = commonValue(doc, unit)
  let overridden = 0
  const effective: unknown[] = []
  for (const memberId of memberIds) {
    const { present, value } = overrideValue(memberRecord(doc, memberId), unit)
    if (present) {
      overridden++
      effective.push(value)
    } else {
      effective.push(common)
    }
  }
  const first = effective[0]
  const uniform = effective.every((v) => sameUnitValue(v, first))
  const source: UnitSource =
    overridden === 0 ? 'common' : overridden === memberIds.length && uniform ? 'member' : 'mixed'
  return {
    value: uniform ? first : undefined,
    source,
    overridden,
    members: memberIds.length,
  }
}

/** Reads participation of a member target: single member or batch summary. */
export function readParticipation(
  doc: OperationDraftDocument,
  target: EditTarget,
): 'participate' | 'exclude' | 'mixed' {
  const memberIds = target.kind === 'member' ? [target.memberId] : target.memberIds
  let excluded = 0
  for (const memberId of memberIds) {
    if (memberRecord(doc, memberId)?.excluded) excluded++
  }
  if (excluded === 0) return 'participate'
  if (excluded === memberIds.length) return 'exclude'
  return 'mixed'
}

function setOverride(record: DraftMember, unit: OverrideUnit, value: unknown): void {
  const overrides: OverrideSet = record.overrides ?? {}
  switch (unit) {
    case 'mode':
      overrides.mode = value as OverrideSet['mode']
      break
    case 'no_effect_tags':
    case 'other_tags':
      overrides[unit] = value as string[]
      break
    case 'matched':
      overrides.matched = value as OverrideSet['matched']
      break
    case 'other':
      overrides.other = value as OverrideSet['other']
      break
    case 'unmatched':
      overrides.unmatched = value as OverrideSet['unmatched']
      break
  }
  record.overrides = overrides
}

function clearOverride(record: DraftMember, unit: OverrideUnit): void {
  if (!record.overrides) return
  switch (unit) {
    case 'mode':
      delete record.overrides.mode
      break
    case 'no_effect_tags':
    case 'other_tags':
      delete record.overrides[unit]
      break
    case 'matched':
      delete record.overrides.matched
      break
    case 'other':
      delete record.overrides.other
      break
    case 'unmatched':
      delete record.overrides.unmatched
      break
  }
  if (Object.keys(record.overrides).length === 0) delete record.overrides
}

function recordFor(doc: OperationDraftDocument, memberId: string): DraftMember {
  let record = memberRecord(doc, memberId)
  if (!record) {
    record = { member_id: memberId }
    doc.members.push(record)
  }
  return record
}

/** Drops records that state nothing: participation plus no override. */
function pruneMembers(doc: OperationDraftDocument): void {
  doc.members = doc.members.filter((m) => m.excluded === true || (m.overrides && Object.keys(m.overrides).length > 0))
}

/**
 * Builds the document to save from the persisted base plus the session's
 * intents. The base is never mutated; only units with a `set`/`inherit` intent
 * change, so this holds: editing one unit leaves every other value and every
 * other member's inheritance relationship byte-identical.
 */
export function applyIntent(
  base: OperationDraftDocument,
  target: EditTarget,
  intent: EditIntent,
): OperationDraftDocument {
  const doc: OperationDraftDocument = {
    ...base,
    no_effect_tags: [...(base.no_effect_tags ?? [])],
    other_tags: [...(base.other_tags ?? [])],
    members: base.members.map((m) => ({
      member_id: m.member_id,
      excluded: m.excluded,
      overrides: m.overrides ? { ...m.overrides } : undefined,
    })),
  }

  const memberIds = target.kind === 'member' ? [target.memberId] : target.memberIds
  for (const memberId of memberIds) {
    if (intent.participation !== 'keep') {
      const record = recordFor(doc, memberId)
      if (intent.participation === 'exclude') record.excluded = true
      else delete record.excluded
    }
    for (const unit of OVERRIDE_UNITS) {
      const unitIntent = intent.units[unit]
      if (!unitIntent || unitIntent.intent === 'keep') continue
      if (unitIntent.intent === 'set') {
        setOverride(recordFor(doc, memberId), unit, unitIntent.value)
      } else {
        const record = memberRecord(doc, memberId)
        if (record) clearOverride(record, unit)
      }
    }
  }
  pruneMembers(doc)
  return doc
}

/** True when the intent would write anything at all. */
export function intentIsEmpty(intent: EditIntent): boolean {
  if (intent.participation !== 'keep') return false
  return OVERRIDE_UNITS.every((unit) => {
    const unitIntent = intent.units[unit]
    return !unitIntent || unitIntent.intent === 'keep'
  })
}

/** How many units an intent sets or restores — the K in "N folders, K settings". */
export function intentUnitCount(intent: EditIntent): number {
  return OVERRIDE_UNITS.filter((unit) => {
    const unitIntent = intent.units[unit]
    return unitIntent && unitIntent.intent !== 'keep'
  }).length
}
