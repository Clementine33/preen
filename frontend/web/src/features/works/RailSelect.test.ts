import type { RailOption } from './RailSelect.vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import RailSelect from './RailSelect.vue'

/**
 * A history's voice actors and circles run to about a thousand, so the rail's
 * list is virtualized: only the rows in view exist, and the keyboard still
 * walks the whole list by index. jsdom has no layout and no scrolling, so the
 * scroller reports the panel's size and `scrollTo` moves it the way a browser
 * would, scroll event included.
 */

const descriptors = {
  offsetHeight: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetHeight')!,
  offsetWidth: Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetWidth')!,
}
/** The stubs below live on HTMLElement; deleting them re-exposes Element's own. */
const shadowed = ['scrollTop', 'scrollHeight', 'clientHeight'] as const
const nativeScrollTo = HTMLElement.prototype.scrollTo
beforeAll(() => {
  const isList = (el: HTMLElement) => el.classList.contains('rs-list')
  Object.defineProperties(HTMLElement.prototype, {
    offsetHeight: {
      configurable: true,
      get(this: HTMLElement) {
        return isList(this)
          ? 320
          : 0
      },
    },
    offsetWidth: {
      configurable: true,
      get(this: HTMLElement) {
        return isList(this)
          ? 280
          : 0
      },
    },
    clientHeight: {
      configurable: true,
      get(this: HTMLElement) {
        return isList(this)
          ? 320
          : 0
      },
    },
    // The virtualizer clamps a jump to scrollHeight - clientHeight.
    scrollHeight: {
      configurable: true,
      get(this: HTMLElement) {
        return isList(this)
          ? Number.parseFloat((this.firstElementChild as HTMLElement | null)?.style.height ?? '0')
          : 0
      },
    },
  })
  // jsdom keeps scrollTop at 0, so the scroller remembers its own.
  const tops = new WeakMap<HTMLElement, number>()
  Object.defineProperty(HTMLElement.prototype, 'scrollTop', {
    configurable: true,
    get(this: HTMLElement) { return tops.get(this) ?? 0 },
    set(this: HTMLElement, value: number) { tops.set(this, value) },
  })
  HTMLElement.prototype.scrollTo = function (this: HTMLElement, options?: ScrollToOptions | number) {
    if (typeof options === 'object' && options.top !== undefined)
      this.scrollTop = options.top
    this.dispatchEvent(new Event('scroll'))
  } as typeof HTMLElement.prototype.scrollTo
})
afterAll(() => {
  Object.defineProperties(HTMLElement.prototype, descriptors)
  for (const name of shadowed) delete (HTMLElement.prototype as unknown as Record<string, unknown>)[name]
  HTMLElement.prototype.scrollTo = nativeScrollTo
})

const OPTIONS: RailOption[] = [
  { value: '', label: 'ALL' },
  ...Array.from({ length: 1000 }, (_, i) => ({ value: `va-${i}`, label: `声優${String(i).padStart(4, '0')}`, count: 1 })),
]

async function mountSelect(initial = '', searchable = true) {
  const value = ref(initial)
  const wrapper = mount(
    defineComponent(() => () =>
      h(RailSelect, {
        'label': 'VOICE',
        'options': OPTIONS,
        searchable,
        'marksActive': true,
        'modelValue': value.value,
        'onUpdate:modelValue': (next: string) => (value.value = next),
      }),
    ),
    {
      attachTo: document.body.appendChild(document.createElement('div')),
    },
  )
  await flushPromises()
  return { wrapper, value }
}

const frame = () => new Promise(resolve => requestAnimationFrame(() => resolve(undefined)))
const focusedIndex = () => Number((document.activeElement as HTMLElement).dataset.index ?? -1)

describe('railSelect', () => {
  it('draws only the rows in view of a thousand options', async () => {
    const { wrapper } = await mountSelect()

    const rows = wrapper.findAll('[role="option"]')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThan(30)
    // Every row still says where it sits in the whole list.
    expect(rows[1].attributes('aria-setsize')).toBe('1001')
  })

  it('opens on the selected option even when it lies far down the list', async () => {
    // A list without a search box focuses its current option on open.
    const { wrapper } = await mountSelect('va-700', false)
    await wrapper.get('.rs-trigger').trigger('click')
    await flushPromises()
    for (let i = 0; i < 6; i++) await frame()

    expect(focusedIndex()).toBe(701)
    expect(document.activeElement?.getAttribute('aria-selected')).toBe('true')
  })

  it('presents the panel as a bottom sheet on a narrow screen and keeps `open` in step', async () => {
    const real = window.matchMedia
    window.matchMedia = ((query: string) => ({
      ...real(query),
      matches: query.includes('max-width'),
    })) as typeof real
    try {
      const { wrapper } = await mountSelect('', false)
      const sheet = wrapper.get('dialog.rs-menu')
      expect(sheet.attributes('open')).toBeUndefined()

      await wrapper.get('.rs-trigger').trigger('click')
      expect(sheet.attributes('open')).toBeDefined()
      expect(wrapper.get('.rs').classes()).toContain('open')

      // The dialog's own close (Esc, back) closes the select; so does CLOSE.
      await sheet.trigger('close')
      expect(wrapper.get('.rs').classes()).not.toContain('open')
      await wrapper.get('.rs-trigger').trigger('click')
      await wrapper.get('.rs-close').trigger('click')
      expect(sheet.attributes('open')).toBeUndefined()

      // A tap on the backdrop is a click on the dialog itself.
      await wrapper.get('.rs-trigger').trigger('click')
      await sheet.trigger('click')
      expect(wrapper.get('.rs').classes()).not.toContain('open')
    }
    finally {
      window.matchMedia = real
    }
  })
})
