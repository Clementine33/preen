import { mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
// The shell derives the current entry from the router's own route name,
// so the test drives the real route table. The slot stands in for the page:
// AppShell renders no RouterView, so no page component is mounted here.
import { router } from '@/app/router'
import AppShell from './AppShell.vue'

function mountShell(): VueWrapper {
  return mount(AppShell, {
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: apiStub() },
    },
    slots: { default: '<div data-testid="page">page</div>' },
  })
}

async function at(path: string): Promise<void> {
  await router.push(path)
  await router.isReady()
}

beforeEach(() => {
  localStorage.clear()
})

describe('AppShell global navigation', () => {
  it('renders the same entries, in the same order and with the same targets, in rail and bottom bar', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    for (const nav of ['global-rail', 'global-bottom-bar'] as const) {
      const links = wrapper.get(`[data-testid="${nav}"]`).findAll('a')
      expect(links.map((link) => link.text())).toEqual(['作品库', '工作集'])
      expect(links.map((link) => link.attributes('href'))).toEqual(['/works', '/worksets'])
    }
  })

  it('switches rail and bottom bar in CSS at the shared 641px breakpoint', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    const rail = wrapper.get('[data-testid="global-rail"]')
    expect(rail.classes()).toContain('hidden')
    expect(rail.classes()).toContain('rail:flex')
    expect(wrapper.get('[data-testid="global-bottom-bar"]').classes()).toContain('rail:hidden')
  })

  it('keeps the brand a non-navigating mark with an accessible product name', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    const brand = wrapper.get('[data-testid="app-brand"]')
    expect(brand.text()).toContain('Onsei Organizer')
    expect(brand.find('a').exists()).toBe(false)
  })

  it('renders no media-library list as global navigation', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    expect(wrapper.text()).not.toContain('媒体库条目')
    const railNav = wrapper.get('[data-testid="global-rail"] nav')
    expect(railNav.findAll('a')).toHaveLength(2)
    expect(railNav.find('button').exists()).toBe(false)
  })

  it('marks the matching entry current and only that one, per route', async () => {
    await at('/worksets')
    let links = mountShell().get('[data-testid="global-rail"]').findAll('a')
    expect(links.map((link) => link.attributes('aria-current'))).toEqual([undefined, 'page'])

    await at('/works')
    links = mountShell().get('[data-testid="global-rail"]').findAll('a')
    expect(links.map((link) => link.attributes('aria-current'))).toEqual(['page', undefined])
  })

  it('keeps 工作集 current inside a library workbench', async () => {
    await at('/worksets/lib-a')
    const links = mountShell().get('[data-testid="global-rail"]').findAll('a')

    expect(links.map((link) => link.attributes('aria-current'))).toEqual([undefined, 'page'])
  })

  it('keeps 工作集 current on a workbench child route', async () => {
    await at('/worksets/lib-a/conversion/settings')
    const links = mountShell().get('[data-testid="global-bottom-bar"]').findAll('a')

    expect(links.map((link) => link.attributes('aria-current'))).toEqual([undefined, 'page'])
  })

  it('carries the only theme entry on the rail, never on the bottom bar', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    // The entry names the mode in force (the default is dark).
    const entry = wrapper.get('[data-testid="global-rail"] [data-testid="theme-menu"]')
    expect(entry.attributes('aria-label')).toBe('切换主题（当前：深色）')
    expect(entry.text()).toBe('DARK')
    expect(wrapper.find('[data-testid="global-bottom-bar"] [data-testid="theme-menu"]').exists()).toBe(false)
  })
})
