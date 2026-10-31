import { describe, expect, it } from 'vitest'
import { shorterIdSelection } from './id-selection'

const universe = ['a', 'b', 'c', 'd']

describe('shorterIdSelection', () => {
  it('lists the picked ids when half or fewer are picked', () => {
    expect(shorterIdSelection(new Set(['b']), universe)).toEqual({
      all: false,
      ids: ['b'],
    })
    expect(shorterIdSelection(new Set(['a', 'c']), universe)).toEqual({
      all: false,
      ids: ['a', 'c'],
    })
  })

  it('names the exceptions when more than half are picked', () => {
    expect(shorterIdSelection(new Set(['a', 'b', 'd']), universe)).toEqual({
      all: true,
      ids: ['c'],
    })
    expect(shorterIdSelection(new Set(universe), universe)).toEqual({ all: true, ids: [] })
  })

  it('keeps the universe order and ignores ids outside it', () => {
    expect(shorterIdSelection(new Set(['d', 'x', 'a']), universe)).toEqual({
      all: false,
      ids: ['a', 'd'],
    })
  })
})
