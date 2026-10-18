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
  it('renders the entries on the rail, in order and with their targets', async () => {
    await at('/worksets')
    const links = mountShell().get('[data-testid="global-rail"] nav').findAll('a')

    expect(links.map((link) => link.text())).toEqual(['作品库', '合集', '工作集'])
    expect(links.map((link) => link.attributes('href'))).toEqual(['/works', '/collections', '/worksets'])
  })

  it('shows the rail in CSS from the shared 641px breakpoint and leaves the phone to the page', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    const rail = wrapper.get('[data-testid="global-rail"]')
    expect(rail.classes()).toContain('hidden')
    expect(rail.classes()).toContain('rail:flex')
    // Phones navigate through the GlobalMenu in each page header; the shell
    // renders no bottom bar.
    expect(wrapper.findAll('nav')).toHaveLength(1)
  })

  it('keeps the brand a non-navigating mark with an accessible product name', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    const brand = wrapper.get('[data-testid="app-brand"]')
    expect(brand.text()).toContain('Preen')
    expect(brand.find('a').exists()).toBe(false)
  })

  it('renders no media-library list as global navigation', async () => {
    await at('/worksets')
    const wrapper = mountShell()

    expect(wrapper.text()).not.toContain('媒体库条目')
    const railNav = wrapper.get('[data-testid="global-rail"] nav')
    expect(railNav.findAll('a')).toHaveLength(3)
    expect(railNav.find('button').exists()).toBe(false)
  })

  it('sets 设置 apart at the rail foot and marks it current on its page', async () => {
    await at('/settings')
    const settings = mountShell().get('[data-testid="global-settings"]')
    expect(settings.attributes('href')).toBe('/settings')
    expect(settings.attributes('aria-current')).toBe('page')
  })

  it('marks the matching entry current and only that one, per route', async () => {
    await at('/worksets')
    let links = mountShell().get('[data-testid="global-rail"] nav').findAll('a')
    expect(links.map((link) => link.attributes('aria-current'))).toEqual([undefined, undefined, 'page'])

    await at('/works')
    links = mountShell().get('[data-testid="global-rail"] nav').findAll('a')
    expect(links.map((link) => link.attributes('aria-current'))).toEqual(['page', undefined, undefined])
  })

  it('keeps 工作集 current inside a library workbench', async () => {
    await at('/worksets/lib-a')
    const links = mountShell().get('[data-testid="global-rail"] nav').findAll('a')

    expect(links.map((link) => link.attributes('aria-current'))).toEqual([undefined, undefined, 'page'])
  })

  it('keeps 工作集 current on a workbench child route', async () => {
    await at('/worksets/lib-a/conversion/settings')
    const links = mountShell().get('[data-testid="global-rail"] nav').findAll('a')

    expect(links.map((link) => link.attributes('aria-current'))).toEqual([undefined, undefined, 'page'])
  })
})
