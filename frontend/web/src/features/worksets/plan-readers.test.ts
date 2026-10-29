import type { ComponentOutcome, RevisionDetailResponse, WorksetMember } from '@/lib/api/types'
import { describe, expect, it } from 'vitest'
import { frozenConclusion } from '@/test/member-conclusion'
import {
  componentHasUnmetTarget,
  componentOperationCount,
  decisionGroups,
  keepReasonText,
  keptDecisions,
  memberConclusion,
  memberConclusions,
  readComponent,
  resolutionText,
} from './plan-readers'

/** A snapshot as an older backend wrote it: empty collections serialized as null. */
const legacyComponent = {
  component_id: 'cmp-legacy',
  partition: 'matched',
  status: 'ok',
  lanes: null,
  variant_decisions: null,
  operations: null,
  projected_inventory: null,
  files: null,
} as unknown as ComponentOutcome

describe('plan snapshot readers', () => {
  it('normalizes a legacy snapshot whose collections are null', () => {
    const component = readComponent(legacyComponent)
    expect(component.lanes).toEqual([])
    expect(component.variant_decisions).toEqual([])
    expect(component.operations).toEqual([])
    expect(component.projected_inventory).toEqual([])
    expect(component.files).toEqual([])
    expect(component.operations).toEqual([])
    expect(componentOperationCount(component)).toBe(0)
    expect(componentHasUnmetTarget(component)).toBe(false)
  })

  it('normalizes a nested variant with no decisions', () => {
    const component = readComponent({
      ...legacyComponent,
      variant_decisions: [
        { stem: 'track' },
      ],
    } as unknown as ComponentOutcome)
    expect(component.variant_decisions[0].decisions).toEqual([])
  })

  it('keeps the real content untouched', () => {
    const component = readComponent({
      component_id: 'cmp-1',
      partition: 'unmatched',
      status: 'ok',
      lanes: [
        { lane: 'encoded', decision: 'REBUILD' },
      ],
      variant_decisions: [
        {
          stem: 'track',
          decisions: [
            { path: '/music/track.mp3', resolution: 'keep', reason_code: 'UNMET_TARGET' },
          ],
        },
      ],
      operations: [
        {
          kind: 'encode',
          phase: 'materialize_outputs',
          component_id: 'cmp-1',
          variant_stem: 'track',
          source_path: '/music/track.flac',
        },
      ],
      projected_inventory: ['/music/track.mp3'],
      files: [
        { path: '/music/track.mp3', size: 1, mtime: 2 },
      ],
    } as unknown as ComponentOutcome)

    expect(componentOperationCount(component)).toBe(1)
    expect(componentHasUnmetTarget(component)).toBe(true)
    expect(component.lanes).toHaveLength(1)
  })
})

describe('member conclusion', () => {
  it('uses a short change badge and keeps conversion scope in the detail', () => {
    const input = frozenConclusion('changed')
    input.task!.matched.changes = true
    input.task!.unmatched.applicable = true
    expect(memberConclusion(input)).toMatchObject({ label: '有变化', tone: 'success', filter: 'change' })
    expect(memberConclusion(input).detail).toBe('无音效：将转换；有音效：无需改动')
  })

  it('distinguishes partial unmet components within the same partition', () => {
    expect(memberConclusion(frozenConclusion('partially_unmet'))).toMatchObject({ label: '部分未满足', tone: 'warning', filter: 'warn' })
  })

  it('reports unmet when every component contains an unmet target, even with operations', () => {
    expect(memberConclusion(frozenConclusion('unmet'))).toMatchObject({ label: '未满足', tone: 'warning', filter: 'warn' })
  })

  it('uses source competition or other blockers as the primary filter above warnings', () => {
    expect(memberConclusion(frozenConclusion('source_ambiguous'))).toMatchObject({ label: '源竞争', tone: 'danger', filter: 'source_conflict' })
    expect(memberConclusion(frozenConclusion('source_ambiguous')).detail).toContain('无法唯一选源')
    expect(memberConclusion(frozenConclusion('blocked'))).toMatchObject({ label: '阻塞', filter: 'blocked' })
  })

  it('preserves inactive and missing-input states', () => {
    expect(memberConclusion(frozenConclusion()).label).toBe('无需转换')
    expect(memberConclusion(frozenConclusion('not_applicable')).label).toBe('无适用')
    expect(memberConclusion(frozenConclusion('blocked', { excluded: true, input: 'missing' }))).toMatchObject({ label: '已排除', filter: null })
    expect(memberConclusion(frozenConclusion('blocked', { input: 'none' }))).toMatchObject({ label: '未参与', filter: null })
    expect(memberConclusion(frozenConclusion('blocked', { input: 'missing' }))).toMatchObject({ label: '输入缺失', filter: 'blocked' })
    expect(memberConclusion(frozenConclusion('changed', { input: 'stale' })).label).toBe('有变化')
  })
})

describe('partition facts', () => {
  it('explains independent partition facts supplied by the server', () => {
    const input = frozenConclusion('unmet')
    input.task!.matched.unmet = true
    expect(memberConclusion(input).detail).toContain('无音效：目标未满足，现有文件按可用源保留')
    input.task!.matched.changes = true
    expect(memberConclusion(input).detail).toContain('无音效：目标未满足，其余按可用源转换')
    input.task!.matched.blocked = true
    expect(memberConclusion(input).detail).toBe('无音效：阻塞；有音效：无适用文件')
  })
})

describe('kept files', () => {
  const component = readComponent({
    component_id: 'cmp',
    partition: 'matched',
    status: 'ok',
    operations: [
      {
        kind: 'encode',
        phase: 'materialize_outputs',
        component_id: 'cmp',
        variant_stem: 'track',
        source_path: '/album/track.flac',
        target_path: '/album/track.mp3',
      },
    ] as ComponentOutcome['operations'],
    variant_decisions: [
      {
        stem: 'track',
        decisions: [
          { path: '/album/track.mp3', resolution: 'encode', target_path: '/album/track.mp3' },
          { path: '/album/track.wav', resolution: 'keep', reason_code: 'UNMET_TARGET' },
          { path: '/album/notes.mp3', resolution: 'delete', reason_code: 'OBSOLETE_ENCODED' },
          { path: '/album/other.mp3', resolution: 'keep', reason_code: 'KEEP_ENCODED_SATISFIED' },
        ],
      },
    ],
  } as ComponentOutcome)

  it('reads only the keep conclusions, never the operations', () => {
    expect(keptDecisions(component).map(decision => decision.path)).toEqual([
      '/album/track.wav',
      '/album/other.mp3',
    ])
  })

  it('explains each kept file in the plan\'s own words', () => {
    expect(keepReasonText('UNMET_TARGET')).toBe('目标未满足')
    expect(keepReasonText('KEEP_ENCODED_SATISFIED')).toBe('已满足编码目标')
    expect(keepReasonText('CONFORMANCE_UNCONFIRMED')).toBe('目标符合性未确认')
    expect(keepReasonText(undefined)).toBe('原样保留')
    // A code an older build planned still reads in words, not as a machine code.
    expect(keepReasonText('QUALITY_UNKNOWN')).toBe('质量未知')
    // An unknown code stays readable rather than disappearing.
    expect(keepReasonText('SOME_NEW_CODE')).toBe('SOME_NEW_CODE')
  })
})

describe('decision rows', () => {
  const component = readComponent({
    component_id: 'cmp',
    partition: 'matched',
    status: 'ok',
    operations: [
      {
        kind: 'encode',
        phase: 'materialize_outputs',
        component_id: 'cmp',
        variant_stem: 'track',
        source_path: '/album/track.flac',
        target_path: '/album/track.mp3',
      },
    ] as ComponentOutcome['operations'],
    variant_decisions: [
      {
        stem: 'track',
        decisions: [
          { path: '/album/track.mp3', resolution: 'encode', target_path: '/album/track.mp3' },
          { path: '/album/track.wav', resolution: 'keep', reason_code: 'UNMET_TARGET' },
          { path: '/album/notes.mp3', resolution: 'delete', reason_code: 'OBSOLETE_ENCODED' },
        ],
      },
    ],
  } as ComponentOutcome)

  it('names every file once, with the one thing that happens to it', () => {
    expect(decisionGroups(component, '/album')).toEqual([
      {
        dir: '',
        rows: [
          { resolution: 'encode', name: 'track.mp3', reasonCode: '' },
          { resolution: 'keep', name: 'track.wav', reasonCode: 'UNMET_TARGET' },
          { resolution: 'delete', name: 'notes.mp3', reasonCode: 'OBSOLETE_ENCODED' },
        ],
      },
    ])
  })

  it('gives a subfolder its own group, named once', () => {
    const nested = readComponent({
      component_id: 'cmp',
      partition: 'matched',
      status: 'ok',
      operations: [] as ComponentOutcome['operations'],
      variant_decisions: [
        {
          stem: 'disc',
          decisions: [
            { path: '/album/disc/01.flac', resolution: 'keep', reason_code: 'KEEP_LOSSLESS_TARGET' },
            { path: '/album/disc/02.flac', resolution: 'keep', reason_code: 'KEEP_LOSSLESS_TARGET' },
          ],
        },
      ],
    } as ComponentOutcome)

    expect(decisionGroups(nested, '/album')).toEqual([
      {
        dir: 'disc',
        rows: [
          { resolution: 'keep', name: '01.flac', reasonCode: 'KEEP_LOSSLESS_TARGET' },
          { resolution: 'keep', name: '02.flac', reasonCode: 'KEEP_LOSSLESS_TARGET' },
        ],
      },
    ])
  })

  it('spells a resolution in the plan\'s own words', () => {
    expect(resolutionText('keep')).toBe('保留')
    expect(resolutionText('delete')).toBe('删除')
    expect(resolutionText('encode')).toBe('生成')
    // An unknown resolution stays readable rather than disappearing.
    expect(resolutionText('rewrap')).toBe('rewrap')
  })
})

describe('memberConclusions', () => {
  function record(count: number) {
    const members = Array.from({ length: count }, (_, i) => ({
      member_id: `m-${i}`,
      folder_name: `f${i}`,
      folder_path: `/lib/f${i}`,
      rel_path: `f${i}`,
      dir_id: `m-${i}`,
    })) as WorksetMember[]
    const view = {
      members: members.map((m, i) => ({
        member_id: m.member_id,
        conclusion: frozenConclusion(i === 0
          ? 'changed'
          : i === 1
            ? 'blocked'
            : 'unchanged', {
          member_id: m.member_id,
          excluded: i === 2,
          input: i === 3
            ? 'none'
            : i === 5
              ? 'missing'
              : 'valid',
        }),
      })),
    } as unknown as RevisionDetailResponse
    return { members, view }
  }

  it('reads each member conclusion without roots or component payloads', () => {
    const { members, view } = record(6)
    const out = memberConclusions(view, members)
    expect(out.get('m-0')?.filter).toBe('change')
    expect(out.get('m-1')?.filter).toBe('blocked')
    expect(out.get('m-2')?.label).toBe('已排除')
    expect(out.get('m-3')?.label).toBe('未参与')
    expect(out.get('m-5')).toEqual(memberConclusion(frozenConclusion('unchanged', { input: 'missing' })))
    expect(out.get('m-4')).toEqual(memberConclusion(frozenConclusion()))
  })

  it('has no entries without a plan, and covers every member of a large record', () => {
    expect(memberConclusions(null, record(3).members).size).toBe(0)
    const { members, view } = record(3000)
    expect(memberConclusions(view, members).size).toBe(3000)
  })
})
