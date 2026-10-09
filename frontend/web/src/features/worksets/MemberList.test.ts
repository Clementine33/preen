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
  const rows = () => wrapper.findAll('[data-testid="member-row"]')
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

it('shows a long folder name in full and keeps its actions reachable', async () => {
  const name = '【バイノーラル】'.repeat(15)
  const member = { member_id: 'm', folder_name: name, folder_path: `/lib/${name}`, rel_path: name, dir_id: 'm' }
  const wrapper = mount(MemberList, {
    props: {
      members: [member],
      draft: { schema_version: 1, no_effect_tags: [], matched: {}, unmatched: {}, members: [] },
      selectedIds: new Set<string>(), hiddenSelectedCount: 0, filter: 'all', search: '',
      conclusionFor: () => memberConclusion({ excluded: false, hasRoot: true, rootMissing: false, components: [] }),
    },
  })
  const open = wrapper.get('[data-testid="member-open"]')
  expect(open.text()).toBe(name)
  expect(open.classes()).not.toContain('truncate')
  await open.trigger('click')
  await wrapper.get('[data-testid="member-files"]').trigger('click')
  await wrapper.get('input[aria-label="全选当前筛选结果"]').trigger('change')
  await wrapper.get(`input[aria-label="选择 ${name}"]`).trigger('change')
  expect(wrapper.emitted('open')).toEqual([['m']])
  expect(wrapper.emitted('files')).toEqual([['m']])
  expect(wrapper.emitted('toggleAll')).toEqual([[['m']]])
  expect(wrapper.emitted('toggle')).toEqual([['m']])
  wrapper.unmount()
})
