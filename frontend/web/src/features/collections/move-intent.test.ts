import { describe, expect, it } from 'vitest'
import { landingIndex, moveIntent } from './move-intent'

const rows = ['A', 'B', 'C', 'D']

describe('moveIntent', () => {
  it('ascending: a gap above row B is before B, the bottom is the end', () => {
    expect(moveIntent(rows, 3, 1, 'asc', true)).toEqual({ before: 'B' })
    expect(moveIntent(rows, 0, 4, 'asc', true)).toEqual({ before: null })
  })
  it('descending: a gap below row A is before A, the very top is the end of saved order', () => {
    expect(moveIntent(rows, 3, 2, 'desc', true)).toEqual({ before: 'B' })
    expect(moveIntent(rows, 3, 0, 'desc', true)).toEqual({ before: null })
    expect(moveIntent(rows, 0, 4, 'desc', true)).toEqual({ before: 'D' })
  })
  it('descending bottom of a partly loaded list needs the first saved member', () => {
    expect(moveIntent(rows, 0, 4, 'desc', false)).toEqual({ before: 'first-saved' })
  })
  it('a move onto its own place is nothing', () => {
    expect(moveIntent(rows, 1, 1, 'asc', true)).toBeNull()
    expect(moveIntent(rows, 1, 2, 'desc', true)).toBeNull()
  })
  it('lands where the gap was, shifted by its own departure', () => {
    expect(landingIndex(0, 3)).toBe(2)
    expect(landingIndex(3, 1)).toBe(1)
  })
})
