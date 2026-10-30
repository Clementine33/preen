import type { VueWrapper } from '@vue/test-utils'
import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import { frozenConclusion } from '@/test/member-conclusion'
import MemberList from './MemberList.vue'
import { memberConclusion } from './plan-readers'

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
  const conclusions = {
    source: frozenConclusion('source_ambiguous'),
    missing: frozenConclusion('blocked'),
    partial: frozenConclusion('partially_unmet'),
    changed: frozenConclusion('changed'),
    mixed: frozenConclusion('blocked'),
  }
  const members = Object.keys(conclusions).map(id => ({
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
      conclusionFor: member => memberConclusion(conclusions[member.member_id as keyof typeof conclusions]),
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
      conclusionFor: () => memberConclusion(frozenConclusion('not_applicable')),
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
      conclusionFor: () => memberConclusion(frozenConclusion('not_applicable')),
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

it('offers 执行结果 only for members inside the current execution session', async () => {
  const members = ['a', 'b'].map(id => ({
    member_id: id,
    folder_name: id,
    folder_path: id,
    rel_path: id,
    dir_id: id,
  }))
  async function buttons(executionScope?: string[] | null) {
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
        conclusionFor: () => memberConclusion(frozenConclusion('changed')),
        executionScope,
      },
    })
    await giveHeight(wrapper)
    return wrapper
  }

  // No session for the revision: no button.
  expect((await buttons(null)).findAll('[data-testid="member-execution"]')).toHaveLength(0)
  expect((await buttons(undefined)).findAll('[data-testid="member-execution"]')).toHaveLength(0)
  // A whole-revision run (empty scope) covers every member.
  const whole = await buttons([])
  expect(whole.findAll('[data-testid="member-execution"]')).toHaveLength(2)
  // A scoped run covers only its folders (record-relative paths).
  const scoped = await buttons(['b'])
  const only = scoped.findAll('[data-testid="member-execution"]')
  expect(only).toHaveLength(1)
  expect(only[0]!.attributes('aria-label')).toBe('查看 b 的执行结果')
  await only[0]!.trigger('click')
  expect(scoped.emitted('openExecution')).toEqual([
    ['b'],
  ])
})
