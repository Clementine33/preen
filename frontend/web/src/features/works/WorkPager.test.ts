import { describe, expect, it } from 'vitest'
import { pagerItems } from './WorkPager.vue'

describe('pagerItems', () => {
  it('keeps the ends and a window either side, marking each gap once', () => {
    expect(pagerItems(1, 4)).toEqual([1, 2, 3, 4])
    expect(pagerItems(10, 20)).toEqual([1, null, 8, 9, 10, 11, 12, null, 20])
  })

  it('shows a lone missing page instead of an ellipsis', () => {
    expect(pagerItems(5, 9)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9])
  })

  it('narrows to one neighbour without hiding a gap', () => {
    expect(pagerItems(1, 6, 1)).toEqual([1, 2, null, 6])
    expect(pagerItems(5, 9, 1)).toEqual([1, null, 4, 5, 6, null, 9])
  })
})
