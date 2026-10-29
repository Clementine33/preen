import type { VueWrapper } from '@vue/test-utils'
import type { ComponentOutcome } from '@/lib/api/types'
import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import MemberList from './MemberList.vue'
import { memberConclusion, readComponent } from './plan-readers'

/** jsdom has no layout: give the scroller a height so the virtualizer draws a window. */
async function giveHeight(wrapper: VueWrapper): Promise<void> {
  Object.defineProperties(wrapper.get('[data-testid="member-scroller"]').element, {
    offsetHeight: {
      configurable: true,
      get: () => 600,
    },
    offsetWidth: {
      configurable: true,
      get: () => 800,
    },
    clientHeight: {
      configurable: true,
      get: () => 600,
    },
    clientWidth: {
      configurable: true,
      get: () => 800,
    },
  })
  await flushPromises()
}

it('filters strictly by the displayed status, including folders with overlapping component facts', async () => {
  const plans: Record<string, ComponentOutcome[]> = {
    source: [
      readComponent({
        component_id: 'source',
        partition: 'matched',
        status: 'blocked',
        reason_code: 'SOURCE_AMBIGUOUS',
      } as ComponentOutcome),
    ],
    missing: [
      readComponent({
        component_id: 'missing',
        partition: 'matched',
        status: 'blocked',
        reason_code: 'SOURCE_MISSING',
      } as ComponentOutcome),
    ],
    partial: [
      readComponent({
        component_id: 'change',
        partition: 'matched',
        status: 'ok',
        operations: [
          { kind: 'encode' },
        ],
      } as ComponentOutcome),
    ],
  }
  const unmet = readComponent({
    component_id: 'unmet',
    partition: 'matched',
    status: 'ok',
    variant_decisions: [
      {
        stem: 'track',
        decisions: [
          { path: '/a.mp3', resolution: 'keep', reason_code: 'UNMET_TARGET' },
        ],
      },
    ],
  } as ComponentOutcome)
  plans.changed = [...plans.partial!]
  plans.source!.push(...plans.partial!)
  plans.partial!.push(unmet)
  plans.mixed = [
    ...plans.source!,
    ...plans.missing!,
    unmet,
  ]
  const members = Object.keys(plans).map(id => ({
    member_id: id,
    folder_name: id,
    folder_path: id,
    rel_path: id,
    dir_id: id,
  }))
  const wrapper = mount(MemberList, {
    props: {
      members,
      draft: {
        schema_version: 1,
        no_effect_tags: ['NoSE'],
        matched: {},
        unmatched: {},
        members: [],
      },
      selectedIds: new Set<string>(),
      hiddenSelectedCount: 0,
      filter: 'all',
      search: '',
      conclusionFor: member => memberConclusion({
        excluded: false,
        hasRoot: true,
        rootMissing: false,
        components: plans[member.member_id]!,
      }),
    },
  })
  await giveHeight(wrapper)
  const rows = () => wrapper.findAll('[data-testid="member-row"]')
  expect(rows()[0]?.text()).toContain('源竞争')
  expect(rows()[2]?.text()).toContain('部分未满足')
  for (const [filter, expected] of [
    [
      'source_conflict',
      ['source'],
    ],
    [
      'blocked',
      ['missing', 'mixed'],
    ],
    [
      'warn',
      ['partial'],
    ],
    [
      'change',
      ['changed'],
    ],
  ] as const) {
    await wrapper.setProps({ filter })
    expect(rows()).toHaveLength(expected.length)
    expected.forEach((name, index) => expect(rows()[index]?.text()).toContain(name))
    if (filter === 'change')
      expect(rows().every(row => row.text().includes('有变化'))).toBe(true)
  }
  wrapper.unmount()
})

it('shows a long folder name in full and keeps its actions reachable', async () => {
  const name = '【バイノーラル】'.repeat(15)
  const member = {
    member_id: 'm',
    folder_name: name,
    folder_path: `/lib/${name}`,
    rel_path: name,
    dir_id: 'm',
  }
  const wrapper = mount(MemberList, {
    props: {
      members: [member],
      draft: {
        schema_version: 1,
        no_effect_tags: [],
        matched: {},
        unmatched: {},
        members: [],
      },
      selectedIds: new Set<string>(),
      hiddenSelectedCount: 0,
      filter: 'all',
      search: '',
      conclusionFor: () => memberConclusion({
        excluded: false,
        hasRoot: true,
        rootMissing: false,
        components: [],
      }),
    },
  })
  await giveHeight(wrapper)
  const open = wrapper.get('[data-testid="member-open"]')
  expect(open.text()).toBe(name)
  expect(open.classes()).not.toContain('truncate')
  await open.trigger('click')
  await wrapper.get('[data-testid="member-files"]').trigger('click')
  await wrapper.get('input[aria-label="全选当前筛选结果"]').trigger('change')
  await wrapper.get(`input[aria-label="选择 ${name}"]`).trigger('change')
  expect(wrapper.emitted('open')).toEqual([
    ['m'],
  ])
  expect(wrapper.emitted('files')).toEqual([
    ['m'],
  ])
  expect(wrapper.emitted('toggleAll')).toEqual([
    [
      ['m'],
    ],
  ])
  expect(wrapper.emitted('toggle')).toEqual([
    ['m'],
  ])
  wrapper.unmount()
})

it('draws only a window of a large member set', async () => {
  // jsdom measures every element at 0px, which would put all rows in the window.
  const heightOf = vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(76)
  const members = Array.from({ length: 5000 }, (_, i) => ({
    member_id: `m-${i}`,
    folder_name: `Folder ${i}`,
    folder_path: `/lib/${i}`,
    rel_path: `${i}`,
    dir_id: `m-${i}`,
  }))
  const wrapper = mount(MemberList, {
    props: {
      members,
      draft: {
        schema_version: 1,
        no_effect_tags: [],
        matched: {},
        unmatched: {},
        members: [],
      },
      selectedIds: new Set<string>(),
      hiddenSelectedCount: 0,
      filter: 'all',
      search: '',
      conclusionFor: () => memberConclusion({
        excluded: false,
        hasRoot: true,
        rootMissing: false,
        components: [],
      }),
    },
  })
  await giveHeight(wrapper)
  const drawn = wrapper.findAll('[data-testid="member-row"]').length
  expect(drawn).toBeGreaterThan(0)
  expect(drawn).toBeLessThan(30)
  // Select-all still addresses every visible member, drawn or not.
  await wrapper.get('input[aria-label="全选当前筛选结果"]').trigger('change')
  expect((wrapper.emitted('toggleAll')![0]![0] as string[]).length).toBe(5000)
  heightOf.mockRestore()
  wrapper.unmount()
})
