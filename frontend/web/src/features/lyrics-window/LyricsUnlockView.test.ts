import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import LyricsUnlockView from './LyricsUnlockView.vue'

it('asks the host to unlock when the chip is pressed', async () => {
  const wrapper = mount(LyricsUnlockView)
  await wrapper.get('button').trigger('click')
  expect(wrapper.emitted('unlock')).toHaveLength(1)
})
