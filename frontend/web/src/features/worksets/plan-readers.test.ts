import { describe, expect, it } from 'vitest'
import type { ComponentOutcome } from '@/lib/api/types'
import {
  decisionGroups,
  keepReasonText,
  keptDecisions,
  resolutionText,
  componentHasUnmetTarget,
  componentOperationCount,
  memberConclusion,
  partitionFacts,
  readComponent,
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
      variant_decisions: [{ stem: 'track' }],
    } as unknown as ComponentOutcome)
    expect(component.variant_decisions[0].decisions).toEqual([])
  })

  it('keeps the real content untouched', () => {
    const component = readComponent({
      component_id: 'cmp-1',
      partition: 'unmatched',
      status: 'ok',
      lanes: [{ lane: 'encoded', decision: 'REBUILD' }],
      variant_decisions: [
        {
          stem: 'track',
          decisions: [{ path: '/music/track.mp3', resolution: 'keep', reason_code: 'UNMET_TARGET' }],
        },
      ],
      operations: [{ kind: 'encode', phase: 'materialize_outputs', component_id: 'cmp-1', variant_stem: 'track', source_path: '/music/track.flac' }],
      projected_inventory: ['/music/track.mp3'],
      files: [{ path: '/music/track.mp3', size: 1, mtime: 2 }],
    } as unknown as ComponentOutcome)

    expect(componentOperationCount(component)).toBe(1)
    expect(componentHasUnmetTarget(component)).toBe(true)
    expect(component.lanes).toHaveLength(1)
  })
})

describe('member conclusion', () => {
  const base = { excluded: false, hasRoot: true, rootMissing: false }
  const component = (overrides: Partial<ComponentOutcome> = {}) =>
    readComponent({ component_id: 'cmp', partition: 'matched', status: 'ok', ...overrides } as ComponentOutcome)
  const changing = component({ operations: [{ kind: 'encode' }] as ComponentOutcome['operations'] })
  const unmet = component({
    variant_decisions: [{ stem: 'track', decisions: [{ path: '/a.mp3', resolution: 'keep', reason_code: 'UNMET_TARGET' }] }],
  })
  const source = component({ status: 'blocked', reason_code: 'SOURCE_AMBIGUOUS' })
  const blocked = component({ status: 'blocked', reason_code: 'SOURCE_MISSING' })

  it('uses a short change badge and keeps conversion scope in the detail', () => {
    const result = memberConclusion({ ...base, components: [changing, component({ partition: 'unmatched' })] })
    expect(result.label).toBe('有变化')
    expect(result.tone).toBe('success')
    expect(result.detail).toContain('无音效：将转换')
    expect(result.detail).toContain('有音效：无需改动')
    expect(result.filter).toBe('change')
  })

  it('distinguishes partial unmet components within the same partition', () => {
    for (const other of [changing, component()]) {
      const result = memberConclusion({ ...base, components: [unmet, other] })
      expect(result.label).toBe('部分未满足')
      expect(result.tone).toBe('warning')
      expect(result.filter).toBe('warn')
    }
    expect(memberConclusion({ ...base, components: [unmet, changing] }).filter).toBe('warn')
  })

  it('reports unmet when every component contains an unmet target, even with operations', () => {
    const result = memberConclusion({ ...base, components: [unmet, { ...unmet, operations: changing.operations }] })
    expect(result.label).toBe('未满足')
    expect(result.tone).toBe('warning')
    expect(result.filter).toBe('warn')
  })

  it('uses source competition or other blockers as the primary filter above warnings', () => {
    const competition = memberConclusion({ ...base, components: [source, unmet] })
    expect(competition.label).toBe('源竞争')
    expect(competition.tone).toBe('danger')
    expect(competition.detail).toContain('无法唯一选源')
    expect(competition.filter).toBe('source_conflict')

    const mixed = memberConclusion({ ...base, components: [source, blocked, unmet] })
    expect(mixed.label).toBe('阻塞')
    expect(mixed.filter).toBe('blocked')
  })

  it('preserves inactive and missing-input states', () => {
    expect(memberConclusion({ ...base, components: [component()] }).label).toBe('无需转换')
    expect(memberConclusion({ ...base, components: [] }).label).toBe('无适用')
    expect(memberConclusion({ ...base, components: [source], excluded: true })).toMatchObject({ label: '已排除', filter: null })
    expect(memberConclusion({ ...base, components: [], hasRoot: false })).toMatchObject({ label: '未参与', filter: null })
    expect(memberConclusion({ ...base, components: [], rootMissing: true })).toMatchObject({ label: '输入缺失', filter: 'blocked' })
  })
})

describe('partition facts', () => {
  function component(overrides: Partial<ComponentOutcome>): ComponentOutcome {
    return readComponent({ component_id: 'cmp', partition: 'matched', status: 'ok', ...overrides } as ComponentOutcome)
  }

  it('accumulates independent facts per partition', () => {
    const facts = partitionFacts([
      component({ operations: [{ kind: 'encode' }] as ComponentOutcome['operations'] }),
      component({
        component_id: 'cmp-2',
        status: 'blocked',
        variant_decisions: [{ stem: 'track', decisions: [{ path: '/a.mp3', resolution: 'keep', reason_code: 'UNMET_TARGET' }] }],
      }),
    ])

    expect(facts.matched).toEqual({ applicable: true, blocked: true, unmet: true, changes: true })
    // The partition with no component holds none of those facts.
    expect(facts.unmatched).toEqual({ applicable: false, blocked: false, unmet: false, changes: false })
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
    expect(keptDecisions(component).map((decision) => decision.path)).toEqual([
      '/album/track.wav',
      '/album/other.mp3',
    ])
  })

  it("explains each kept file in the plan's own words", () => {
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

  it("spells a resolution in the plan's own words", () => {
    expect(resolutionText('keep')).toBe('保留')
    expect(resolutionText('delete')).toBe('删除')
    expect(resolutionText('encode')).toBe('生成')
    // An unknown resolution stays readable rather than disappearing.
    expect(resolutionText('rewrap')).toBe('rewrap')
  })
})
