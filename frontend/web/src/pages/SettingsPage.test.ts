import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ApiError, apiClientKey } from '@/lib/api/client'
import { useMotion } from '@/composables/use-motion'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import SettingsPage from './SettingsPage.vue'

enableAutoUnmount(afterEach)
afterEach(() => vi.unstubAllGlobals())

async function mountPage(api: ReturnType<typeof apiStub>, path = '/settings') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/settings/:section(appearance|tags)?', name: 'settings', component: SettingsPage }],
  })
  await router.push(path)
  const wrapper = mount(SettingsPage, {
    attachTo: document.body.appendChild(document.createElement('div')),
    global: { plugins: [createPinia(), router, installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return wrapper
}

describe('settings: logout', () => {
  it('clears the session before replacing the document', async () => {
    let resolveLogout!: () => void
    const deleteSession = vi.fn(() => new Promise<void>((resolve) => { resolveLogout = resolve }))
    const replace = vi.fn()
    vi.stubGlobal('location', { replace })
    const wrapper = await mountPage(apiStub({ deleteSession }))
    await wrapper.get('[data-testid="logout"]').trigger('click')
    expect(replace).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="logout"]').attributes('disabled')).toBeDefined()
    resolveLogout()
    await flushPromises()
    expect(replace).toHaveBeenCalledExactlyOnceWith('/login')
  })

  it('keeps the page open and allows retry when logout fails', async () => {
    const replace = vi.fn()
    vi.stubGlobal('location', { replace })
    const wrapper = await mountPage(apiStub({ deleteSession: vi.fn().mockRejectedValue(new Error('offline')) }))
    await wrapper.get('[data-testid="logout"]').trigger('click')
    await flushPromises()
    expect(replace).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('退出失败')
    expect(wrapper.get('[data-testid="logout"]').attributes('disabled')).toBeUndefined()
  })
})

describe('settings: user tags', () => {
  const tags = [{ id: 't1', name: '睡眠用' }, { id: 't2', name: '雨音' }]

  it('lists the whole vocabulary and keeps the typed name on a rename conflict', async () => {
    const renameUserTag = vi.fn().mockRejectedValue(new ApiError(409, 'USER_TAG_EXISTS', 'exists'))
    const wrapper = await mountPage(apiStub({ listUserTags: vi.fn().mockResolvedValue(tags), renameUserTag }), '/settings/tags')
    expect(wrapper.find('[data-testid="user-tag-t1"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="user-tag-t2"]').exists()).toBe(true)

    await wrapper.get('[data-testid="user-tag-rename-t2"]').trigger('click')
    await wrapper.get('[data-testid="user-tag-rename-input"]').setValue('睡眠用')
    await wrapper.get('[data-testid="user-tag-rename-submit"]').trigger('submit')
    await flushPromises()

    expect(renameUserTag).toHaveBeenCalledWith('t2', '睡眠用')
    expect(wrapper.get('[data-testid="user-tag-rename-error"]').text()).toContain('不会自动合并')
    expect((wrapper.get('[data-testid="user-tag-rename-input"]').element as HTMLInputElement).value).toBe('睡眠用')
  })

  it('deletes a tag only after confirmation and drops it from the list', async () => {
    const deleteUserTag = vi.fn().mockResolvedValue(undefined)
    const wrapper = await mountPage(apiStub({
      listUserTags: vi.fn().mockResolvedValue(tags),
      listTagFacets: vi.fn().mockResolvedValue({ source_tags: [], user_tags: [{ id: 't2', name: '雨音', work_count: 4 }] }),
      deleteUserTag,
    }), '/settings/tags')

    await wrapper.get('[data-testid="user-tag-delete-t2"]').trigger('click')
    await flushPromises()
    expect(deleteUserTag).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('4 部作品上移除')

    document.querySelector<HTMLButtonElement>('[data-testid="user-tag-delete-confirm"]')!.click()
    await flushPromises()
    expect(deleteUserTag).toHaveBeenCalledWith('t2')
    expect(wrapper.find('[data-testid="user-tag-t2"]').exists()).toBe(false)
  })
})

describe('settings: format preference', () => {
  it('saves each edit at once, keeps the order, and rolls back with a toast on failure', async () => {
    const putPreferences = vi.fn(async (input: { track_formats: string[] }) => input)
    const wrapper = await mountPage(apiStub({
      getPreferences: vi.fn().mockResolvedValue({ track_formats: ['WAV', 'FLAC'] }),
      putPreferences,
    }))
    const rows = () => wrapper.findAll('li[data-testid^="format-"]')
    expect(rows().map((row) => row.get('span.font-mono').text())).toEqual(['WAV', 'FLAC'])

    await wrapper.get('[aria-label="FLAC 上移"]').trigger('click')
    await flushPromises()
    expect(putPreferences).toHaveBeenLastCalledWith({ track_formats: ['FLAC', 'WAV'] })

    await wrapper.get('[data-testid="format-add-MP3"]').trigger('click')
    await flushPromises()
    expect(putPreferences).toHaveBeenLastCalledWith({ track_formats: ['FLAC', 'WAV', 'MP3'] })
    expect(wrapper.find('[data-testid="format-add-MP3"]').exists()).toBe(false)

    putPreferences.mockRejectedValueOnce(new ApiError(500, 'INTERNAL', 'boom'))
    await wrapper.get('[aria-label="移除 WAV"]').trigger('click')
    await flushPromises()
    expect(rows().map((row) => row.get('span.font-mono').text())).toEqual(['FLAC', 'WAV', 'MP3'])
  })
})

describe('settings: categories', () => {
  it('opens on appearance and switches category from the sidebar', async () => {
    const wrapper = await mountPage(apiStub({ listUserTags: vi.fn().mockResolvedValue([]) }))
    expect(wrapper.get('[data-testid="settings-nav-appearance"]').attributes('aria-current')).toBe('page')
    expect(wrapper.find('[data-testid="user-tag-settings"]').exists()).toBe(false)

    await wrapper.get('[data-testid="settings-nav-tags"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="settings-nav-tags"]').attributes('aria-current')).toBe('page')
    expect(wrapper.find('[data-testid="user-tag-settings"]').exists()).toBe(true)
  })
})

describe('settings: appearance', () => {
  it('applies and stores the theme', async () => {
    const wrapper = await mountPage(apiStub())
    await wrapper.get('[data-testid="theme-preference"] [data-value="light"]').trigger('click')
    await flushPromises()
    expect(localStorage.getItem('preen-theme')).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('stores the motion choice and marks the document', async () => {
    const wrapper = await mountPage(apiStub())
    await wrapper.get('[data-testid="motion-preference"] [data-value="reduce"]').trigger('click')
    await flushPromises()
    expect(localStorage.getItem('preen-motion')).toBe('reduce')
    expect(document.documentElement.classList.contains('reduce-motion')).toBe(true)
    useMotion().preference.value = 'system'
    await flushPromises()
    expect(document.documentElement.classList.contains('reduce-motion')).toBe(false)
  })
})
