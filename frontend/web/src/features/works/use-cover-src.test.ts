import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { coverEpoch } from '@/queries/cover-epoch'
import { remoteThumb, useCoverSrc } from './use-cover-src'

let last: ReturnType<typeof useCoverSrc>
function mountSrc() {
  mount(defineComponent(() => {
    last = useCoverSrc(() => ({ rj: 'RJ1', remote: 'https://covers.test/RJ1.jpg' }))
    return () => h('div')
  }))
}

afterEach(() => { coverEpoch.value = 0 })

describe('useCoverSrc', () => {
  it('gives null once the remote link fails, and retries after the next list/detail read', () => {
    mountSrc()
    expect(last.src.value).toBe('https://covers.test/RJ1.jpg')
    last.failed('https://covers.test/RJ1.jpg')
    expect(last.src.value).toBeNull()
    coverEpoch.value++
    expect(last.src.value).toBe('https://covers.test/RJ1.jpg')
  })
})

describe('remoteThumb', () => {
  const DLSITE = 'https://img.dlsite.jp'
  it.each([
    ['DLsite jpg', `${DLSITE}/modpub/images2/work/doujin/RJ110000/RJ109959_img_main.jpg`, `${DLSITE}/resize/images2/work/doujin/RJ110000/RJ109959_img_main_240x240.jpg`],
    ['DLsite webp', `${DLSITE}/modpub/images2/work/doujin/RJ110000/RJ109959_img_main.webp`, `${DLSITE}/resize/images2/work/doujin/RJ110000/RJ109959_img_main_240x240.jpg`],
    ['ASMR', 'https://api.asmr.one/api/cover/109959.jpg?type=main&v=2', 'https://api.asmr.one/api/cover/109959.jpg?type=240x240&v=2'],
    ['other host', 'https://covers.test/main.webp', 'https://covers.test/main.webp'],
    ['other DLsite path', `${DLSITE}/other/main.jpg`, `${DLSITE}/other/main.jpg`],
    ['no extension', `${DLSITE}/modpub/main`, `${DLSITE}/modpub/main`],
    ['not an address', 'nonsense', 'nonsense'],
    ['empty', '', null],
    ['unknown', null, null],
  ])('%s', (_, cover, want) => {
    expect(remoteThumb(cover)).toBe(want)
  })
})
