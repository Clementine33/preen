import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import type { Library } from '@/lib/api/types'
import { desktopAdapterKey } from '@/lib/desktop/desktop-adapter'
import LibraryManager from './LibraryManager.vue'

const library: Library = {
  id: 'a', name: '库', root_path: '/m/a', created_at: '', updated_at: '', last_scan_at: null,
  last_scan_status: '', last_scan_error: '', storage_class: 'ssd',
}

const global = {
  provide: { [desktopAdapterKey as symbol]: { platform: 'browser', lyricsWindow: false, openLyricsWindow: async () => null, resolveApi: async () => ({ baseUrl: '', token: null }), pickFolder: async () => null } },
}

afterEach(() => document.body.replaceChildren())
enableAutoUnmount(afterEach)

const attach = async (props: { open: boolean; library?: Library }) => {
  const wrapper = mount(LibraryManager, { props, global, attachTo: document.body })
  await flushPromises()
  return wrapper
}
const ssdInput = () => document.querySelector<HTMLInputElement>('[data-testid="library-ssd"]')!

describe('LibraryManager storage class', () => {
  it('starts from the library and saves the toggled class with the rest', async () => {
    const wrapper = await attach({ open: true, library })
    expect(ssdInput().checked).toBe(true)

    ssdInput().click()
    document.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await flushPromises()
    expect(wrapper.emitted('save')).toEqual([[{ name: '库', root_path: '/m/a', storage_class: 'hdd' }]])
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
