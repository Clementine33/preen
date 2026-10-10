import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { createPinia } from 'pinia'
import { nextTick } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { useLibraryUiStore } from '@/stores/library-ui'
import LibrarySwitcher from './LibrarySwitcher.vue'
import type { Library } from '@/lib/api/types'

/**
 * The library switcher's contracts: it reuses the one
 * library-selection state the workbench already has — switching dispatches
 * `setActiveLibrary` and nothing else — and while the menu is open it owns the
 * keyboard: J/K move inside the menu and never reach the wall underneath.
 */

function library(id: string, name: string, root: string): Library {
  return {
    id,
    name,
    root_path: root,
    created_at: '',
    updated_at: '',
    last_scan_at: null,
    last_scan_status: 'completed',
    last_scan_error: '',
  }
}

const libraries = [library('lib-a', 'Archive', '/music'), library('lib-b', 'Brazil', '/brazil')]

function routerFor(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/worksets', name: 'worksets', component: { template: '<div />' } }],
  })
}

let container: HTMLElement | null = null

async function mountSwitcher(): Promise<VueWrapper> {
  const api = apiStub({
    listLibraries: vi.fn().mockResolvedValue(libraries),
  } as never)
  const router = routerFor()
  await router.push('/worksets')
  await router.isReady()
  container = document.body.appendChild(document.createElement('div'))
  const wrapper = mount(LibrarySwitcher, {
    attachTo: container,
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return wrapper
}

/** jsdom has no PointerEvent constructor; the handler only reads the target. */
function pointerDown(target: EventTarget): void {
  target.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
}

/** A keydown on the switcher's root, as its own handler receives it. */
async function press(wrapper: VueWrapper, key: string): Promise<void> {
  await wrapper.find('.lib').trigger('keydown', { key })
}

beforeEach(() => {
  localStorage.clear()
})
afterEach(() => {
  container?.remove()
  container = null
})

describe('LibrarySwitcher', () => {
  it('shows the active library and opens into a listbox of every root', async () => {
    const wrapper = await mountSwitcher()

    expect(wrapper.get('.lib-name').text()).toBe('Archive')
    await wrapper.get('.lib-trigger').trigger('click')
    await nextTick()

    expect(wrapper.get('.lib-trigger').attributes('aria-expanded')).toBe('true')
    const options = wrapper.findAll('[role="option"]')
    // ALL LIBRARIES leads, then every root.
    expect(options).toHaveLength(3)
    expect(options.map((option) => option.attributes('aria-selected'))).toEqual(['false', 'true', 'false'])
    // The panel states its scale itself: two roots, no per-library work counts.
    expect(wrapper.get('.lib-menu').text()).toContain('2 ROOTS')
  })

  it('switches through the shared store and closes, returning focus to the trigger', async () => {
    const wrapper = await mountSwitcher()
    await wrapper.get('.lib-trigger').trigger('click')
    await nextTick()

    await wrapper.findAll('[role="option"]')[2].trigger('click')
    await nextTick()

    const ui = useLibraryUiStore()
    expect(ui.activeLibraryId).toBe('lib-b')
    expect(wrapper.get('.lib-trigger').attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('.lib-name').text()).toBe('Brazil')
    expect(document.activeElement).toBe(wrapper.get('.lib-trigger').element)
  })

  it('offers the ALL scope ahead of the roots', async () => {
    const wrapper = await mountSwitcher()
    await wrapper.get('.lib-trigger').trigger('click')
    await nextTick()

    await wrapper.findAll('[role="option"]')[0].trigger('click')
    await nextTick()

    expect(useLibraryUiStore().allLibraries).toBe(true)
    expect(wrapper.get('.lib-name').text()).toBe('ALL')
    expect(wrapper.findAll('[role="option"]').map((option) => option.attributes('aria-selected')))
      .toEqual(['true', 'false', 'false'])
  })

  it('moves inside the menu with J/K and stops the events from reaching the wall', async () => {
    const wrapper = await mountSwitcher()
    const onWallKey = vi.fn()
    wrapper.vm.$el.parentElement?.addEventListener('keydown', onWallKey)

    await wrapper.get('.lib-trigger').trigger('click')
    await nextTick()
    // Opening focuses the current library, ready for J/K.
    expect(document.activeElement?.getAttribute('aria-selected')).toBe('true')

    await press(wrapper, 'j')
    expect(document.activeElement?.textContent).toContain('Brazil')

    await press(wrapper, 'k')
    expect(document.activeElement?.textContent).toContain('Archive')

    // The wall's J/K never fire underneath an open menu.
    expect(onWallKey).not.toHaveBeenCalled()
  })

  it('closes on Escape and returns focus to the trigger', async () => {
    const wrapper = await mountSwitcher()
    await wrapper.get('.lib-trigger').trigger('click')
    await nextTick()

    await press(wrapper, 'Escape')
    await nextTick()

    expect(wrapper.get('.lib-trigger').attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(wrapper.get('.lib-trigger').element)
  })

  it('closes on a pointerdown outside and ignores one inside', async () => {
    const wrapper = await mountSwitcher()
    await wrapper.get('.lib-trigger').trigger('click')
    await nextTick()
    expect(wrapper.get('.lib-trigger').attributes('aria-expanded')).toBe('true')

    // Inside: the panel stays open.
    pointerDown(wrapper.get('.lib-menu').element)
    await nextTick()
    expect(wrapper.get('.lib-trigger').attributes('aria-expanded')).toBe('true')

    // Outside: it closes.
    pointerDown(document.body)
    await nextTick()
    expect(wrapper.get('.lib-trigger').attributes('aria-expanded')).toBe('false')
  })
})
