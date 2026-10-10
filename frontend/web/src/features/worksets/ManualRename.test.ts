import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ManualRename from './ManualRename.vue'
import type { WorksetMember } from '@/lib/api/types'

function members(count: number): WorksetMember[] {
  return Array.from({ length: count }, (_, i) => {
    const code = `RJ${String(i + 1).padStart(8, '0')}`
    return { member_id: `m-${i}`, folder_path: `/lib/${code} x`, folder_name: `${code} x`, rel_path: `${code} x`, dir_id: `d-${i}` }
  })
}

function mountList(count = 3000, records: { member_id: string; excluded: boolean; name?: string }[] = []) {
  return mount(ManualRename, { props: { members: members(count), records, disabled: false, saving: false } })
}

describe('ManualRename', () => {
  it('draws nothing until the user searches, and never more than twenty matches', async () => {
    const wrapper = mountList()
    expect(wrapper.find('[data-testid="manual-rename-matches"]').exists()).toBe(false)
    await wrapper.get('[data-testid="manual-rename-search"]').setValue('rj0000')
    expect(wrapper.get('[data-testid="manual-rename-matches"]').findAll('li')).toHaveLength(20)
  })

  it('finds a folder by its full work code directly', async () => {
    const wrapper = mountList()
    await wrapper.get('[data-testid="manual-rename-search"]').setValue('RJ00002500')
    const rows = wrapper.get('[data-testid="manual-rename-matches"]').findAll('li')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('RJ00002500 x')
  })

  it('saves a new name on submit, cancels on Esc or the cancel button, never on blur, and treats an untouched name as no decision', async () => {
    const wrapper = mountList(5)
    await wrapper.get('[data-testid="manual-rename-search"]').setValue('rj00000003')
    await wrapper.get('[data-testid="manual-rename-matches"] button').trigger('click')
    const input = wrapper.get('[data-testid="manual-rename-input"]')
    expect((input.element as HTMLInputElement).value).toBe('RJ00000003 x')

    await wrapper.get('[data-testid="manual-rename-form"]').trigger('submit')
    expect(wrapper.emitted('save')).toBeUndefined()

    await wrapper.get('[data-testid="manual-rename-matches"] button').trigger('click')
    await wrapper.get('[data-testid="manual-rename-input"]').setValue('[RJ00000003] mine')
    await wrapper.get('[data-testid="manual-rename-input"]').trigger('keydown.esc')
    expect(wrapper.emitted('save')).toBeUndefined()

    await wrapper.get('[data-testid="manual-rename-matches"] button').trigger('click')
    await wrapper.get('[data-testid="manual-rename-input"]').setValue('[RJ00000003] mine')
    await wrapper.get('[data-testid="manual-rename-input"]').trigger('blur')
    expect(wrapper.emitted('save')).toBeUndefined()
    expect(wrapper.find('[data-testid="manual-rename-form"]').exists()).toBe(true)
    await wrapper.get('[data-testid="manual-rename-cancel"]').trigger('click')
    expect(wrapper.find('[data-testid="manual-rename-form"]').exists()).toBe(false)

    await wrapper.get('[data-testid="manual-rename-matches"] button').trigger('click')
    await wrapper.get('[data-testid="manual-rename-input"]').setValue('[RJ00000003] mine')
    await wrapper.get('[data-testid="manual-rename-form"]').trigger('submit')
    expect(wrapper.emitted('save')).toEqual([['m-2', '[RJ00000003] mine']])
  })

  it('lists the names already given and clears one', async () => {
    const wrapper = mountList(5, [{ member_id: 'm-1', excluded: false, name: '[RJ00000002] mine' }])
    const named = wrapper.get('[data-testid="manual-rename-named"]')
    expect(named.text()).toContain('RJ00000002 x → [RJ00000002] mine')
    await named.get('button').trigger('click')
    expect(wrapper.emitted('save')).toEqual([['m-1', '']])
  })

  it('stops listing a name once the folder carries it', () => {
    const wrapper = mountList(5, [
      { member_id: 'm-1', excluded: false, name: 'RJ00000002 x' },
      { member_id: 'm-2', excluded: false, name: '[RJ00000003] mine' },
    ])
    const text = wrapper.get('[data-testid="manual-rename-named"]').text()
    expect(text).toContain('已单独命名 1')
    expect(text).not.toContain('RJ00000002 x →')
  })
})
