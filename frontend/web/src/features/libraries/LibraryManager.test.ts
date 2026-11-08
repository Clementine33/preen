import type { Library } from '@/lib/api/types'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { desktopAdapterKey } from '@/lib/desktop/desktop-adapter'
import LibraryManager from './LibraryManager.vue'

const library: Library = {
  id: 'a',
  name: '库',
  root_path: '/m/a',
  created_at: '',
  updated_at: '',
  last_scan_at: null,
  last_scan_status: '',
  last_scan_error: '',
  storage_class: 'ssd',
}

const global = {
  provide: {
    [desktopAdapterKey as symbol]: {
      platform: 'browser',
      lyricsWindow: false,
      openLyricsWindow: async () => null,
      resolveApi: async () => ({ baseUrl: '', token: null }),
      pickFolder: async () => null,
    },
  },
}

afterEach(() => document.body.replaceChildren())
enableAutoUnmount(afterEach)

async function attach(props: { open: boolean, library?: Library, isDefault?: boolean }) {
  const wrapper = mount(LibraryManager, { props, global, attachTo: document.body })
  await flushPromises()
  return wrapper
}
const ssdInput = () => document.querySelector<HTMLInputElement>('[data-testid="library-ssd"]')!

describe('libraryManager storage class', () => {
  it('starts from the library and saves the toggled class with the rest', async () => {
    const wrapper = await attach({ open: true, library })
    expect(ssdInput().checked).toBe(true)

    ssdInput().click()
    document.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await flushPromises()
    expect(wrapper.emitted('save')).toEqual([
      [
        {
          name: '库',
          root_path: '/m/a',
          storage_class: 'hdd',
          is_default: false,
        },
      ],
    ])
  })

  it('locks the root of an existing library and leaves a new one editable', async () => {
    await attach({ open: true, library })
    expect(document.querySelector<HTMLInputElement>('[data-testid="library-root"]')!.readOnly).toBe(true)
    expect(document.body.textContent).not.toContain('选择文件夹')
    await attach({ open: true })
    expect(document.querySelectorAll<HTMLInputElement>('[data-testid="library-root"]')[1].readOnly).toBe(false)
  })

  it('defaults a new library to HDD', async () => {
    await attach({ open: true })
    expect(ssdInput().checked).toBe(false)
  })

  it('emits close on Escape', async () => {
    const wrapper = await attach({ open: true })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})

describe('libraryManager default library', () => {
  const defaultInput = () => document.querySelector<HTMLInputElement>('[data-testid="library-default"]')

  it('starts from whether the library is the default and saves the toggled choice', async () => {
    const wrapper = await attach({ open: true, library, isDefault: true })
    expect(defaultInput()!.checked).toBe(true)

    defaultInput()!.click()
    document.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await flushPromises()
    expect(wrapper.emitted('save')).toEqual([
      [
        {
          name: '库',
          root_path: '/m/a',
          storage_class: 'ssd',
          is_default: false,
        },
      ],
    ])
  })

  it('offers no default choice for a library that does not exist yet', async () => {
    const wrapper = await attach({ open: true })
    expect(defaultInput()).toBeNull()

    document.querySelector<HTMLInputElement>('#library-name')!.value = 'x'
    document.querySelector<HTMLInputElement>('#library-name')!.dispatchEvent(new Event('input'))
    document.querySelector<HTMLInputElement>('[data-testid="library-root"]')!.value = '/x'
    document.querySelector<HTMLInputElement>('[data-testid="library-root"]')!.dispatchEvent(new Event('input'))
    document.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await flushPromises()
    expect(wrapper.emitted('save')![0]![0]).not.toHaveProperty('is_default')
  })
})
