import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import type { TagFacets } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import TagIndex, { type TagSelection } from './TagIndex.vue'

/**
 * The tag index's contracts (ADR 0012 §4): source and user tags merge by name,
 * each section is ordered by its own vocabulary's count, and the dialog only
 * stages — APPLY commits, any other way out discards. The dialog is portalled,
 * so it is read from the document rather than the wrapper.
 */

const FACETS: TagFacets = {
  source_tags: [
    { id: 's-ear', name: '掏耳', work_count: 3 },
    { id: 's-asmr', name: 'ASMR', work_count: 9 },
    { id: 's-rare', name: '治愈', work_count: 1 },
  ],
  user_tags: [
    { id: 'u-asmr', name: 'ASMR', work_count: 1 },
    { id: 'u-mine', name: 'mine', work_count: 4 },
  ],
}

async function mountIndex(initial: TagSelection = { groups: [], mode: 'all' }, listTagFacets = vi.fn().mockResolvedValue(FACETS)) {
  const selection = ref<TagSelection>(initial)
  const Host = defineComponent(() => () =>
    h(TagIndex, {
      scope: {},
      modelValue: selection.value,
      'onUpdate:modelValue': (value: TagSelection) => (selection.value = value),
    }),
  )
  const wrapper = mount(Host, {
    attachTo: document.body.appendChild(document.createElement('div')),
    global: {
      plugins: [installTestQueryPlugin()],
      provide: { [apiClientKey as symbol]: apiStub({ listTagFacets } as never) },
    },
  })
  await flushPromises()
  await wrapper.get('.ti-trigger').trigger('click')
  await flushPromises()
  return { wrapper, selection, listTagFacets }
}

const dialog = () => document.querySelector<HTMLElement>('[role="dialog"]')
const sectionChips = (title: string) =>
  [...document.querySelectorAll<HTMLElement>('.ti-section')]
    .find((section) => section.querySelector('.strip-name')?.textContent === title)!
    .querySelectorAll<HTMLButtonElement>('.ti-chips .chip')
const chip = (title: string, name: string) =>
  [...sectionChips(title)].find((button) => button.firstChild?.textContent === name)!
const button = (label: string) =>
  [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find((b) => b.textContent === label)!

afterEach(() => {
  document.body.innerHTML = ''
})

describe('TagIndex', () => {
  it('merges the vocabularies by name and orders each section by its own count', async () => {
    await mountIndex()

    expect([...sectionChips('作品标签')].map((b) => b.textContent)).toEqual(['ASMR9', '掏耳3', '治愈1'])
    expect([...sectionChips('我的标签')].map((b) => b.textContent)).toEqual(['mine4', 'ASMR1'])

    // The same name is one entry: picking it in one section picks it in both.
    chip('作品标签', 'ASMR').click()
    await flushPromises()
    expect(chip('我的标签', 'ASMR').getAttribute('aria-pressed')).toBe('true')
  })

  it('commits the picks with every id of each name and the mode on APPLY', async () => {
    const { selection } = await mountIndex()

    chip('作品标签', 'ASMR').click()
    chip('作品标签', '掏耳').click()
    button('OR').click()
    await flushPromises()
    // Staged only: nothing reached the page yet.
    expect(selection.value.groups).toEqual([])

    button('APPLY').click()
    await flushPromises()
    expect(selection.value).toEqual({ groups: [['s:s-asmr', 'u:u-asmr'], ['s:s-ear']], mode: 'any' })
    expect(dialog()).toBeNull()
  })

  it('discards staged picks when closed without APPLY', async () => {
    const initial: TagSelection = { groups: [['u:u-mine']], mode: 'all' }
    const { wrapper, selection } = await mountIndex(initial)

    // Opening starts from the applied selection.
    expect(chip('我的标签', 'mine').getAttribute('aria-pressed')).toBe('true')
    chip('作品标签', '治愈').click()
    button('OR').click()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[aria-label="关闭"]')!.click()
    await flushPromises()

    expect(selection.value).toEqual(initial)
    // Reopening forgets the discarded draft.
    await wrapper.get('.ti-trigger').trigger('click')
    await flushPromises()
    expect(chip('作品标签', '治愈').getAttribute('aria-pressed')).toBe('false')
    expect(button('AND').getAttribute('aria-checked')).toBe('true')
  })

  it('narrows both sections by name', async () => {
    await mountIndex()

    const find = document.querySelector<HTMLInputElement>('[aria-label="Find tag"]')!
    find.value = 'asm'
    find.dispatchEvent(new Event('input'))
    await flushPromises()

    expect([...sectionChips('作品标签')].map((b) => b.firstChild?.textContent)).toEqual(['ASMR'])
    expect([...sectionChips('我的标签')].map((b) => b.firstChild?.textContent)).toEqual(['ASMR'])
  })

  it('keeps a pick the scoped lists no longer carry, named from the library and removable', async () => {
    // The wall's other filters leave no work with 治愈 in scope; the library has it.
    const listTagFacets = vi.fn((scope: { library?: string; voice_actor?: string }) =>
      Promise.resolve(scope.voice_actor ? { source_tags: [], user_tags: [] } : FACETS))
    const selection = ref<TagSelection>({ groups: [['s:s-rare'], ['s:s-gone']], mode: 'all' })
    const Host = defineComponent(() => () =>
      h(TagIndex, {
        scope: { library: 'lib-a', voice_actor: 'va-1' },
        modelValue: selection.value,
        'onUpdate:modelValue': (value: TagSelection) => (selection.value = value),
      }),
    )
    const wrapper = mount(Host, {
      attachTo: document.body.appendChild(document.createElement('div')),
      global: {
        plugins: [installTestQueryPlugin()],
        provide: { [apiClientKey as symbol]: apiStub({ listTagFacets } as never) },
      },
    })
    await flushPromises()
    expect(wrapper.get('.ti-value').text()).toBe('2 · AND')
    await wrapper.get('.ti-trigger').trigger('click')
    await flushPromises()

    const picked = () => [...document.querySelectorAll<HTMLButtonElement>('.ti-picked .chip')]
    expect(picked().map((b) => b.firstChild?.textContent)).toEqual(['治愈', '未知标签'])
    // APPLY without touching them keeps both; removing one drops only that one.
    button('APPLY').click()
    await flushPromises()
    expect(selection.value.groups).toEqual([['s:s-rare'], ['s:s-gone']])

    await wrapper.get('.ti-trigger').trigger('click')
    await flushPromises()
    picked()[1].click()
    await flushPromises()
    button('APPLY').click()
    await flushPromises()
    expect(selection.value.groups).toEqual([['s:s-rare']])
  })
})
