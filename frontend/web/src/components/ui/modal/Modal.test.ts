import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { h, ref } from 'vue'
import Modal from './Modal.vue'
import Sheet from './Sheet.vue'

afterEach(() => document.body.replaceChildren())
enableAutoUnmount(afterEach)

describe.each([['Modal', Modal], ['Sheet', Sheet]] as const)('%s accessibility', (_, component) => {
  it.each([undefined, 'Dialog instructions'])('labels the dialog with description %s and restores the background on Escape', async (description) => {
    const background = document.body.appendChild(document.createElement('button'))
    background.textContent = 'Background'
    const open = ref(true)
    mount({
      render: () => h(component, {
        open: open.value,
        title: 'Dialog title',
        description,
        'onUpdate:open': (value: boolean) => { open.value = value },
        onClose: () => { open.value = false },
      }),
    }, { attachTo: document.body })
    await flushPromises()

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toBe('Dialog title')
    if (description) {
      expect(document.getElementById(dialog.getAttribute('aria-describedby')!)?.textContent).toBe(description)
    } else {
      expect(dialog.hasAttribute('aria-describedby')).toBe(false)
    }
    expect(background.getAttribute('aria-hidden')).toBe('true')

    dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()

    expect(open.value).toBe(false)
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(background.hasAttribute('aria-hidden')).toBe(false)
  })
})
