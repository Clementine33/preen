import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SettingsApplyBar from './SettingsApplyBar.vue'
import SettingsField from './SettingsField.vue'

function mountField(props: Record<string, unknown> = {}) {
  return mount(SettingsField, {
    props: { id: 'f', label: '转换模式', ...props },
    slots: { default: '<input data-testid="control" />' },
  })
}

describe('settingsField', () => {
  it('disables 撤销 while a save is in flight', () => {
    const wrapper = mountField({ modified: true, disabled: true })
    expect(wrapper.get('[data-testid="f-undo"]').attributes('disabled')).toBeDefined()
  })
})

describe('settingsApplyBar', () => {
  it('locks both buttons while applying', () => {
    const wrapper = mount(SettingsApplyBar, {
      props: { applying: true, error: null },
    })
    expect(wrapper.get('[data-testid="settings-apply"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="settings-discard"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})
