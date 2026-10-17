import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { Library } from '@/lib/api/types'
import { desktopAdapterKey } from '@/lib/desktop/desktop-adapter'
import LibraryManager from './LibraryManager.vue'

const library: Library = {
  id: 'a', name: '库', root_path: '/m/a', created_at: '', updated_at: '', last_scan_at: null,
  last_scan_status: '', last_scan_error: '', storage_class: 'ssd',
}

const global = {
  provide: { [desktopAdapterKey as symbol]: { platform: 'browser', resolveApi: async () => ({ baseUrl: '', token: null }), pickFolder: async () => null } },
}

describe('LibraryManager storage class', () => {
  it('starts from the library and saves the toggled class with the rest', async () => {
    const wrapper = mount(LibraryManager, { props: { open: true, library }, global })
    const ssd = wrapper.get('[data-testid="library-ssd"]')
    expect((ssd.element as HTMLInputElement).checked).toBe(true)

    await ssd.setValue(false)
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('save')).toEqual([[{ name: '库', root_path: '/m/a', storage_class: 'hdd' }]])
  })

  it('defaults a new library to HDD', () => {
    const wrapper = mount(LibraryManager, { props: { open: true }, global })
    expect((wrapper.get('[data-testid="library-ssd"]').element as HTMLInputElement).checked).toBe(false)
  })
})
