import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { coverEpoch } from '@/queries/cover-epoch'
import { useCoverSrc } from './use-cover-src'

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
