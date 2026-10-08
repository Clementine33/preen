import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick, reactive } from 'vue'
import type { OperationDraftDocument } from '@/lib/api/types'
import { OVERRIDE_UNITS } from '@/features/worksets/draft-intents'
import { useWorksetEditorStore } from '@/stores/workset-editor'
import CommonSettingsForm from './CommonSettingsForm.vue'
import UnitSelect from './UnitSelect.vue'

const DEFAULT_TAGS = ['SEなし']

function draft(overrides: Partial<OperationDraftDocument> = {}): OperationDraftDocument {
  return reactive({
    schema_version: 1,
    mode: 'strict',
    no_effect_tags: ['自定义'],
    matched: { encoded: { codec: 'mp3', quality: { kind: 'bitrate', bitrate: 192 } } },
    unmatched: { lostless: undefined, lossless: { codec: 'wav' } },
    members: [],
    ...overrides,
  }) as unknown as OperationDraftDocument
}

function mountForm(harness: OperationDraftDocument) {
  const store = useWorksetEditorStore()
  store.open({
    worksetId: 'ws-1',
    operation: 'conversion',
    target: { kind: 'common' },
    baseVersion: 1,
    baseDocument: harness,
  })
  return mount(CommonSettingsForm, { props: { draft: harness, defaultTags: DEFAULT_TAGS } })
}

describe('CommonSettingsForm', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('offers the fields directly instead of an intent switch', () => {
    const wrapper = mountForm(draft())
    // The values are the persisted ones, editable in place.
    expect(wrapper.findComponent(UnitSelect).props('value')).toBe('strict')
    expect(wrapper.get('[data-testid="common-no_effect_tags"]').text()).toContain('自定义')
    expect(wrapper.find('[data-testid="common-matched-encoded"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="unit-mode-set"]').exists()).toBe(false)
  })

  it('writes a common value when a field changes', async () => {
    const harness = draft()
    const wrapper = mountForm(harness)
    expect(wrapper.get('[data-testid="common-group-mode"]').text()).toContain('严格模式要求')
    expect(wrapper.get('[data-testid="common-group-mode"]').text()).not.toContain('可用源模式按音轨处理')

    wrapper.findAllComponents(UnitSelect)
      .find(select => select.vm.$attrs['data-testid'] === 'common-mode')!.vm.$emit('change', 'available_sources')
    await nextTick()

    expect(wrapper.get('[data-testid="common-group-mode"]').text()).toContain('可用源模式按音轨处理')
    expect(wrapper.get('[data-testid="common-group-mode"]').text()).not.toContain('严格模式要求')
    expect(useWorksetEditorStore().session?.intent.units.mode).toEqual({
      intent: 'set',
      value: 'available_sources',
    })
    // The draft itself is untouched.
    expect(harness.mode).toBe('strict')
  })

  it('restores the seeded default of one group', async () => {
    const wrapper = mountForm(draft())

    await wrapper.get('[data-testid="common-mode-restore"]').trigger('click')
    await nextTick()

    expect(useWorksetEditorStore().session?.intent.units.mode).toEqual({ intent: 'set', value: 'available_sources' })

    await wrapper.get('[data-testid="common-no_effect_tags-restore"]').trigger('click')
    await nextTick()
    expect(useWorksetEditorStore().session?.intent.units.no_effect_tags).toEqual({
      intent: 'set',
      value: DEFAULT_TAGS,
    })

    await wrapper.get('[data-testid="common-matched-restore"]').trigger('click')
    await nextTick()
    expect(useWorksetEditorStore().session?.intent.units.matched).toEqual({
      intent: 'set',
      value: {
        encoded: { codec: 'mp3', quality: { kind: 'bitrate', bitrate: 320 } },
      },
    })
  })

  it('marks a group as changed and offers 全部恢复默认', async () => {
    // Start from the seeded default so the restore action is offered as a
    // no-op first and becomes meaningful once the value changes.
    const wrapper = mountForm(draft({ mode: 'available_sources' }))
    expect(wrapper.get('[data-testid="common-mode-restore"]').attributes('disabled')).toBeDefined()

    wrapper.findAllComponents(UnitSelect)
      .find(select => select.vm.$attrs['data-testid'] === 'common-mode')!.vm.$emit('change', 'strict')
    await nextTick()

    expect(wrapper.text()).toContain('已修改')
    expect(wrapper.find('[data-testid="restore-all-defaults"]').exists()).toBe(true)

    await wrapper.get('[data-testid="restore-all-defaults"]').trigger('click')
    await nextTick()
    // Every group now carries its default as an explicit common value.
    const units = useWorksetEditorStore().session?.intent.units ?? {}
    for (const unit of OVERRIDE_UNITS) {
      expect(units[unit]?.intent).toBe('set')
    }
    expect(useWorksetEditorStore().session?.intent.units.mode).toEqual({ intent: 'set', value: 'available_sources' })
  })

  it('never writes member records', async () => {
    const wrapper = mountForm(draft())
    wrapper.findAllComponents(UnitSelect)
      .find(select => select.vm.$attrs['data-testid'] === 'common-mode')!.vm.$emit('change', 'available_sources')
    await nextTick()
    expect(useWorksetEditorStore().pendingDocument?.members).toEqual([])
  })

  it('edits the obsolete-audio handling as a whole-operation choice', async () => {
    const wrapper = mountForm(draft())
    // Soft deletion is the seeded default: no warning, restore is a no-op.
    expect((wrapper.get('[data-testid="common-delete-mode-soft"]').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.find('[data-testid="common-hard-delete-warning"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="common-delete_mode-restore"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="common-delete-mode-hard"]').setValue(true)
    await nextTick()

    expect(wrapper.get('[data-testid="common-hard-delete-warning"]').text()).toContain('不可恢复')
    expect(useWorksetEditorStore().session?.intent.deleteMode).toEqual({ intent: 'set', value: 'hard' })
    expect(useWorksetEditorStore().pendingDocument?.delete_mode).toBe('hard')

    await wrapper.get('[data-testid="common-delete_mode-restore"]').trigger('click')
    await nextTick()
    expect(useWorksetEditorStore().session?.intent.deleteMode).toEqual({ intent: 'set', value: 'soft' })
  })
  it('warns for empty no-effect tags while keeping the two rule defaults separate', async () => {
    const wrapper = mountForm(draft({ no_effect_tags: [], other_tags: ['xxx'] }))
    await wrapper.setProps({ tagLibrary: {
      default_no_effect_tags: ['NoSE'], default_other_tags: ['反転'], custom_tags: [],
    } })
    expect(wrapper.text()).toContain('可以保存草稿')
    await wrapper.get('[data-testid="common-other_tags-restore"]').trigger('click')
    expect(useWorksetEditorStore().pendingDocument?.other_tags).toEqual(['反転'])
    expect(useWorksetEditorStore().pendingDocument?.no_effect_tags).toEqual([])
  })

})
