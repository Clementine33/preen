import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { lyricsWindowKey } from '@/composables/use-lyrics-window'
import PlayerBar from './PlayerBar.vue'

function render(lyrics?: object) {
  return mount(PlayerBar, {
    global: {
      plugins: [
        createPinia(),
      ],
      stubs: { RouterLink: true },
      provide: lyrics
        ? { [lyricsWindowKey as symbol]: lyrics }
        : {},
    },
  })
}

it('hides the Lyrics Window button when the host cannot show one', () => {
  expect(render().find('.bl-lyrics').exists()).toBe(false)
  expect(render({
    supported: false,
    open: ref(false),
    locked: ref(false),
    available: ref(true),
    toggle: vi.fn(),
  }).find('.bl-lyrics').exists()).toBe(false)
})

it('disables the button without Timed Lyrics and toggles otherwise', async () => {
  const toggle = vi.fn()
  const available = ref(false)
  const wrapper = render({
    supported: true,
    open: ref(false),
    locked: ref(false),
    available,
    toggle,
  })
  expect(wrapper.get('.bl-lyrics').attributes('disabled')).toBeDefined()
  available.value = true
  await wrapper.vm.$nextTick()
  await wrapper.get('.bl-lyrics').trigger('click')
  expect(toggle).toHaveBeenCalled()
})

it('turns the button into an unlock while the window is locked', () => {
  const wrapper = render({
    supported: true,
    open: ref(true),
    locked: ref(true),
    available: ref(true),
    toggle: vi.fn(),
  })
  expect(wrapper.get('.bl-lyrics').attributes('aria-label')).toBe('Unlock lyrics window')
})
