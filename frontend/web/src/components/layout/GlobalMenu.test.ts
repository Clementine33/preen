import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
// The menu derives the current entry from the router's own route name, so the
// test drives the real route table, as the AppShell test does.
import { router } from '@/app/router'
import GlobalMenu from './GlobalMenu.vue'

async function openAt(path: string): Promise<VueWrapper> {
  await router.push(path)
  await router.isReady()
  const wrapper = mount(GlobalMenu, {
    global: { plugins: [router] },
    attachTo: document.body.appendChild(document.createElement('div')),
  })
  await wrapper.get('[data-testid="global-menu"]').trigger('keydown', { key: 'Enter' })
  await flushPromises()
  return wrapper
}

function radios(): HTMLElement[] {
  return [...document.body.querySelectorAll<HTMLElement>('[role="menuitemradio"]')]
}

beforeEach(() => {
  localStorage.clear()
  document.body.innerHTML = ''
})

describe('GlobalMenu', () => {
  it('exists only below the rail breakpoint', async () => {
    const wrapper = await openAt('/worksets')

    expect(wrapper.get('[data-testid="global-menu"]').classes()).toContain('rail:hidden')
  })

  it('offers the global entries, then the theme modes, marking the current entry on a child route', async () => {
    const wrapper = await openAt('/worksets/lib-a/conversion/settings')

    expect(wrapper.get('[data-testid="global-menu"]').attributes('aria-label')).toBe('全局导航（当前：工作集）')
    expect(radios().map((item) => item.textContent?.trim())).toEqual(['作品库', '合集', '工作集', '浅色', '深色', '跟随系统'])
    expect(radios().map((item) => item.getAttribute('aria-checked'))).toEqual(['false', 'false', 'true', 'false', 'true', 'false'])
  })

  it('navigates to the chosen entry', async () => {
    await openAt('/worksets')

    radios()[0].click()

    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/works'))
  })
})
