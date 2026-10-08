import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import type { ComponentOutcome } from '@/lib/api/types'
import { memberConclusion, readComponent } from './plan-readers'
import MemberList from './MemberList.vue'

it('filters strictly by the displayed status, including folders with overlapping component facts', async () => {
  const plans: Record<string, ComponentOutcome[]> = {
    source: [readComponent({ component_id: 'source', partition: 'matched', status: 'blocked', reason_code: 'SOURCE_AMBIGUOUS' } as ComponentOutcome)],
    missing: [readComponent({ component_id: 'missing', partition: 'matched', status: 'blocked', reason_code: 'SOURCE_MISSING' } as ComponentOutcome)],
    partial: [readComponent({ component_id: 'change', partition: 'matched', status: 'ok', operations: [{ kind: 'encode' }] } as ComponentOutcome)],
  }
  const unmet = readComponent({ component_id: 'unmet', partition: 'matched', status: 'ok', variant_decisions: [
    { stem: 'track', decisions: [{ path: '/a.mp3', resolution: 'keep', reason_code: 'UNMET_TARGET' }] },
  ] } as ComponentOutcome)
  plans.changed = [...plans.partial!]
  plans.source!.push(...plans.partial!)
  plans.partial!.push(unmet)
  plans.mixed = [...plans.source!, ...plans.missing!, unmet]
  const members = Object.keys(plans).map((id) => ({ member_id: id, folder_name: id, folder_path: id, rel_path: id, dir_id: id }))
  const wrapper = mount(MemberList, {
    props: {
      members,
      draft: { schema_version: 1, no_effect_tags: ['NoSE'], matched: {}, unmatched: {}, members: [] },
      selectedIds: new Set<string>(), hiddenSelectedCount: 0, filter: 'all', search: '',
      conclusionFor: (member) => memberConclusion({ excluded: false, hasRoot: true, rootMissing: false, components: plans[member.member_id]! }),
    },
  })
  const rows = () => wrapper.findAll('tbody tr')
  expect(rows()[0]?.text()).toContain('源竞争')
  expect(rows()[2]?.text()).toContain('部分未满足')
  for (const [filter, expected] of [
    ['source_conflict', ['source']],
    ['blocked', ['missing', 'mixed']],
    ['warn', ['partial']],
    ['change', ['changed']],
  ] as const) {
    await wrapper.setProps({ filter })
    expect(rows()).toHaveLength(expected.length)
    expected.forEach((name, index) => expect(rows()[index]?.text()).toContain(name))
    if (filter === 'change') expect(rows().every((row) => row.text().includes('有变化'))).toBe(true)
  }
  wrapper.unmount()
})
