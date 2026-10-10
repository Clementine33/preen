import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TaskPlanTable from './TaskPlanTable.vue'
import type { RenameComponent, WorksetMember } from '@/lib/api/types'

const component: RenameComponent = {
  member_id: 'm-1', source_name: 'RJ01 old', work_code: 'RJ01', raw_name: '', target_name: '[RJ01] new', action: 'rename',
}
const member: WorksetMember = {
  member_id: 'm-1', folder_path: '/lib/分类/RJ01 old', folder_name: 'RJ01 old', rel_path: '分类/RJ01 old', dir_id: 'd',
}

describe('TaskPlanTable', () => {
  it('shows the shared parent on both lines so a nested rename visibly stays in place', () => {
    const rows = mount(TaskPlanTable, { props: { components: [component], members: [member] } }).findAll('li')
    expect(rows).toHaveLength(1)
    const [oldLine, newLine] = rows[0].findAll('span.truncate')
    expect(oldLine.text()).toContain('分类/RJ01 old')
    expect(newLine.text()).toBe('↘ 分类/[RJ01] new')
  })

  it('draws no parent for a top-level folder', () => {
    const top = { ...member, rel_path: 'RJ01 old' }
    const wrapper = mount(TaskPlanTable, { props: { components: [component], members: [top] } })
    expect(wrapper.findAll('span.truncate')[1].text()).toBe('↘ [RJ01] new')
  })
})
