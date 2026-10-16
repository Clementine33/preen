import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SettingsApplyBar from './SettingsApplyBar.vue'
import SettingsField from './SettingsField.vue'
import SettingsHead from './SettingsHead.vue'

const mountField = (props: Record<string, unknown> = {}) =>
  mount(SettingsField, { props: { id: 'f', label: '转换模式', ...props }, slots: { default: '<input data-testid="control" />' } })

describe('SettingsField', () => {
  it('shows nothing about an untouched field', () => {
    const wrapper = mountField({ modified: false })
    expect(wrapper.find('[data-testid="f-modified"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="f-undo"]').exists()).toBe(false)
  })

  it('marks an unsaved edit beside 撤销 and emits undo', async () => {
    const wrapper = mountField({ modified: true })
    expect(wrapper.get('[data-testid="f-modified"]').text()).toBe('已修改')
    await wrapper.get('[data-testid="f-undo"]').trigger('click')
    expect(wrapper.emitted('undo')).toHaveLength(1)
  })

  it('disables 撤销 while a save is in flight', () => {
    const wrapper = mountField({ modified: true, disabled: true })
    expect(wrapper.get('[data-testid="f-undo"]').attributes('disabled')).toBeDefined()
  })

  it('folds into one column by container width, not by viewport', () => {
    const wrapper = mountField()
    expect(wrapper.classes()).toContain('@container')
    expect(wrapper.html()).toContain('@lg:grid-cols-')
  })
})

describe('SettingsHead', () => {
  it('offers the one 恢复默认 only while some field is not at its default', async () => {
    const off = mount(SettingsHead, { props: { scope: '所有重命名记录共用', canRestore: false } })
    expect(off.get('[data-testid="settings-restore-defaults"]').attributes('disabled')).toBeDefined()
    const on = mount(SettingsHead, { props: { scope: '所有重命名记录共用', canRestore: true } })
    await on.get('[data-testid="settings-restore-defaults"]').trigger('click')
    expect(on.emitted('restore')).toHaveLength(1)
  })
})

describe('SettingsApplyBar', () => {
  it('says 应用 and shows a refusal beside the buttons', async () => {
    const wrapper = mount(SettingsApplyBar, { props: { applying: false, error: '另一条记录正在执行' } })
    expect(wrapper.get('[data-testid="settings-apply"]').text()).toBe('应用')
    expect(wrapper.get('[role="alert"]').text()).toBe('另一条记录正在执行')
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await wrapper.get('[data-testid="settings-discard"]').trigger('click')
    expect(wrapper.emitted('apply')).toHaveLength(1)
    expect(wrapper.emitted('discard')).toHaveLength(1)
  })

  it('locks both buttons while applying', () => {
    const wrapper = mount(SettingsApplyBar, { props: { applying: true, error: null } })
    expect(wrapper.get('[data-testid="settings-apply"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="settings-discard"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})
