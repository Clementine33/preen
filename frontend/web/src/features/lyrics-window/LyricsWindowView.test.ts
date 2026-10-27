import type { LyricsFrame } from '@/lib/desktop/desktop-adapter'
import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import LyricsWindowView from './LyricsWindowView.vue'

function frame(over: Partial<LyricsFrame> = {}): LyricsFrame {
  return {
    rj: 'RJ1',
    title: 'Track',
    paused: false,
    hasPrev: true,
    hasNext: true,
    line: null,
    noLyrics: false,
    ...over,
  }
}
function show(over?: Partial<LyricsFrame>, slots = {}) {
  return mount(LyricsWindowView, {
    props: {
      frame: frame(over),
    },
    slots,
    global: {
      stubs: { Transition: false },
    },
  })
}

it('shows the current line, and a two-line cue as a main and a smaller sub line', () => {
  const wrapper = show({
    line: { key: 'k', text: 'こんにちは\n你好\nignored' },
  })
  expect(wrapper.findAll('.main').map(n => n.text())).toEqual(['こんにちは'])
  expect(wrapper.findAll('.sub').map(n => n.text())).toEqual(['你好'])
})

it('dims a held cue', () => {
  expect(show({
    line: { key: 'k', text: 'a', dim: true },
  }).find('.line').classes()).toContain('dim')
})

it('shows the title before the first line, with 无歌词 when the track has none', () => {
  expect(show().find('.line').text()).toBe('Track')
  expect(show().find('.none').exists()).toBe(false)
  expect(show({ noLyrics: true }).find('.none').text()).toBe('无歌词')
})

it('disables prev and next by the frame and labels play/pause by state', () => {
  const wrapper = show({ hasPrev: false, hasNext: false, paused: true })
  const [prev, toggle, next] = wrapper.findAll('button')
  expect(prev.attributes('disabled')).toBeDefined()
  expect(next.attributes('disabled')).toBeDefined()
  expect(toggle.attributes('aria-label')).toBe('Resume')
})

it('emits the three commands', async () => {
  const wrapper = show()
  for (const button of wrapper.findAll('button')) await button.trigger('click')
  expect(wrapper.emitted('command')).toEqual([
    ['prev'],
    ['toggle'],
    ['next'],
  ])
})

it('renders host-supplied buttons in the bar and leaves it with the three commands otherwise', () => {
  expect(show().findAll('.bar button')).toHaveLength(3)
  const wrapper = show({}, { host: '<button type="button" class="host">x</button>' })
  expect(wrapper.findAll('.bar button')).toHaveLength(4)
  expect(wrapper.find('.bar .host').exists()).toBe(true)
})
