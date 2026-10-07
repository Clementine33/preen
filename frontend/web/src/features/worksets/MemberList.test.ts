import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import MemberList from './MemberList.vue'

it('filters source conflicts independently of other blocked members', () => {
  const members = ['source', 'missing', 'okay'].map((id) => ({ member_id: id, folder_name: id, folder_path: id, rel_path: id, dir_id: id }))
  const wrapper = mount(MemberList, {
    props: {
      members,
      draft: { schema_version: 1, no_effect_tags: ['NoSE'], matched: {}, unmatched: {}, members: [] },
      selectedIds: new Set<string>(), hiddenSelectedCount: 0, filter: 'source_conflict', search: '',
      conclusionFor: () => ({ tone: 'danger' as const, label: '阻塞', detail: '' }),
      sourceConflictFor: (member: { member_id: string }) => member.member_id === 'source',
    },
  })
  const rows = wrapper.findAll('tbody tr')
  expect(rows).toHaveLength(1)
  expect(rows[0]?.text()).toContain('source')
})
