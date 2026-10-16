import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { h } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, RouterView, type Router } from 'vue-router'
import { ApiError, apiClientKey } from '@/lib/api/client'
import type { ApiClientContract, ConversionSettings } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import SettingsSegmented from '@/features/settings/SettingsSegmented.vue'
import ConversionSettingsPage from './ConversionSettingsPage.vue'

const MP3_320 = { encoded: { codec: 'mp3', quality: { kind: 'bitrate' as const, bitrate: 320 } } }

// The stored settings: not the defaults, so 恢复默认 has something to restore.
const stored: ConversionSettings = {
  mode: 'strict',
  no_effect_tags: ['A'],
  other_tags: [],
  matched: MP3_320,
  unmatched: MP3_320,
}

/** No record is registered anywhere: the page must stand on the settings alone. */
function pageApi(overrides: Partial<ApiClientContract> = {}): ApiClientContract {
  return apiStub({
    getTaskSettings: vi.fn(async () => ({ settings: stored, etag: '"e1"' })) as never,
    saveTaskSettings: vi.fn(async (_type: string, settings: unknown) => ({ settings, etag: '"e2"' })) as never,
    getCurrentRecord: vi.fn().mockResolvedValue({ workset: null }),
    listClassifierTags: vi.fn().mockResolvedValue({ default_no_effect_tags: ['SEなし'], default_other_tags: [], custom_tags: [] }),
    ...overrides,
  } as never)
}

async function mountSettings(overrides: Partial<ApiClientContract> = {}): Promise<{ wrapper: VueWrapper; router: Router; api: ApiClientContract }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/worksets/:libraryId/conversion', name: 'conversion', component: { template: '<div />' } },
      { path: '/worksets/:libraryId/conversion/settings', name: 'conversion-settings', component: ConversionSettingsPage },
    ],
  })
  await router.push('/worksets/lib-a/conversion/settings')
  await router.isReady()
  const api = pageApi(overrides)
  const wrapper = mount({ render: () => h(RouterView) }, {
    global: {
      plugins: [createPinia(), router, installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, router, api }
}

const modeSelect = (wrapper: VueWrapper) =>
  wrapper.findAllComponents(SettingsSegmented).find((select) => select.vm.$attrs['data-testid'] === 'common-mode')!
async function pickMode(wrapper: VueWrapper, value: string) {
  modeSelect(wrapper).vm.$emit('change', value)
  await flushPromises()
}

enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())

describe('ConversionSettingsPage', () => {
  it('shows the stored settings in a library with no record', async () => {
    const { wrapper, api } = await mountSettings()
    expect(wrapper.text()).not.toContain('加载中')
    expect(api.getTaskSettings).toHaveBeenCalledWith('conversion', expect.anything())
    expect(api.getOperationDraft).not.toHaveBeenCalled()
    expect(modeSelect(wrapper).exists()).toBe(true)
    expect(wrapper.get('[data-testid="common-no_effect_tags"]').text()).toContain('A')
    // Nothing is edited yet: no marker, no apply bar.
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="common-group-mode-modified"]').exists()).toBe(false)
  })

  it('marks an unsaved edit, undoes it one field at a time, and leaves the others alone', async () => {
    const { wrapper } = await mountSettings()
    await pickMode(wrapper, 'available_sources')
    expect(wrapper.find('[data-testid="common-group-mode-modified"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="common-group-no_effect_tags-modified"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(true)

    await wrapper.get('[data-testid="common-group-mode-undo"]').trigger('click')
    expect(wrapper.find('[data-testid="common-group-mode-modified"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(false)
  })

  it('does not call setting a field back to its saved value a change', async () => {
    const { wrapper } = await mountSettings()
    await pickMode(wrapper, 'available_sources')
    await pickMode(wrapper, 'strict')
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(false)
  })

  it('restores every default from the one 恢复默认, as unsaved edits only where a field differs', async () => {
    const { wrapper } = await mountSettings()
    // mode is saved as strict, the default is available_sources; the delete mode is already soft.
    const restore = wrapper.get('[data-testid="settings-restore-defaults"]')
    expect(restore.attributes('disabled')).toBeUndefined()

    await restore.trigger('click')
    expect(wrapper.find('[data-testid="common-group-mode-modified"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="common-group-delete_mode-modified"]').exists()).toBe(false)
    expect(modeSelect(wrapper).props('value')).toBe('available_sources')
    expect(wrapper.get('[data-testid="settings-restore-defaults"]').attributes('disabled')).toBeDefined()
  })

  it('applies to every record of the kind, without counts in the bar', async () => {
    const { wrapper, api } = await mountSettings()
    await pickMode(wrapper, 'available_sources')
    const apply = wrapper.get('[data-testid="settings-apply"]')
    expect(apply.text()).toBe('应用')
    expect(wrapper.get('[data-testid="settings-apply-bar"]').text()).not.toMatch(/\d/)

    await apply.trigger('click')
    await flushPromises()
    // The saved object goes back with only the edited field changed.
    expect(api.saveTaskSettings).toHaveBeenCalledWith('conversion', { ...stored, mode: 'available_sources' }, '"e1"')
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="common-group-mode-modified"]').exists()).toBe(false)
    expect(modeSelect(wrapper).exists()).toBe(true)
  })

  it('discards every edit from the apply bar', async () => {
    const { wrapper, api } = await mountSettings()
    await pickMode(wrapper, 'available_sources')
    await wrapper.get('[data-testid="settings-discard"]').trigger('click')
    expect(wrapper.find('[data-testid="settings-apply-bar"]').exists()).toBe(false)
    expect(modeSelect(wrapper).props('value')).toBe('strict')
    expect(api.saveTaskSettings).not.toHaveBeenCalled()
  })

  it.each([
    ['COMMON_SETTINGS_BUSY', '另一条转换记录正在生成或执行'],
    ['COMMON_SETTINGS_UNREADABLE', '草稿无法读取'],
    ['VERSION_CONFLICT', '已在别处被修改'],
    ['INVALID_SETTINGS', 'the settings are not valid'],
  ])('keeps the edits and explains a refused save (%s) in the apply bar', async (code, text) => {
    const { wrapper } = await mountSettings({
      saveTaskSettings: vi.fn().mockRejectedValue(new ApiError(409, code, 'the settings are not valid')) as never,
    })
    await pickMode(wrapper, 'available_sources')
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="settings-apply-error"]').text()).toContain(text)
    expect(wrapper.find('[data-testid="common-group-mode-modified"]').exists()).toBe(true)
    expect(modeSelect(wrapper).props('value')).toBe('available_sources')
  })

  it('reloads the settings after a version conflict so the next apply carries the new ETag', async () => {
    const getTaskSettings = vi.fn()
      .mockResolvedValueOnce({ settings: stored, etag: '"e1"' })
      .mockResolvedValue({ settings: { ...stored, no_effect_tags: ['B'] }, etag: '"e9"' })
    const saveTaskSettings = vi.fn()
      .mockRejectedValueOnce(new ApiError(409, 'VERSION_CONFLICT', 'stale'))
      .mockImplementation(async (_type: string, settings: unknown) => ({ settings, etag: '"e10"' }))
    const { wrapper } = await mountSettings({ getTaskSettings, saveTaskSettings } as never)
    await pickMode(wrapper, 'available_sources')
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="settings-apply"]').trigger('click')
    await flushPromises()

    expect(saveTaskSettings).toHaveBeenLastCalledWith(
      'conversion',
      { ...stored, no_effect_tags: ['B'], mode: 'available_sources' },
      '"e9"',
    )
  })

  it('says why the hard delete mode is dangerous and never marks "not default" as modified', async () => {
    const { wrapper } = await mountSettings()
    await wrapper.get('[data-testid="common-delete-mode-hard"]').setValue(true)
    expect(wrapper.find('[data-testid="common-hard-delete-warning"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="common-group-delete_mode-modified"]').exists()).toBe(true)
    await wrapper.get('[data-testid="common-group-delete_mode-undo"]').trigger('click')
    expect(wrapper.find('[data-testid="common-hard-delete-warning"]').exists()).toBe(false)
  })

  it('reports a failed read with a retry instead of hanging', async () => {
    const { wrapper } = await mountSettings({
      getTaskSettings: vi.fn().mockRejectedValue(new ApiError(500, 'INTERNAL', 'boom')) as never,
    })
    expect(wrapper.get('[role="alert"]').text()).toContain('设置读取失败')
    expect(wrapper.text()).not.toContain('加载中')
  })
})

describe('ConversionSettingsPage route leave', () => {
  it.each([false, true])('confirms unsaved edits and respects the answer %s', async (accept) => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(accept)
    const { wrapper, router } = await mountSettings()
    await pickMode(wrapper, 'available_sources')

    await router.push('/worksets/lib-a/conversion')
    await flushPromises()

    expect(confirm).toHaveBeenCalledOnce()
    expect(router.currentRoute.value.path).toBe(accept ? '/worksets/lib-a/conversion' : '/worksets/lib-a/conversion/settings')
  })

  it('leaves without asking when nothing is unsaved', async () => {
    const confirm = vi.spyOn(window, 'confirm')
    const { router } = await mountSettings()
    await router.push('/worksets/lib-a/conversion')
    await flushPromises()
    expect(confirm).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/worksets/lib-a/conversion')
  })
})
