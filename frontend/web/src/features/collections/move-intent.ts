// The arithmetic of moving a row on the screen into a saved-order `before_rj`.
// A gap `g` (0..n) lies above screen row g; the screen is the saved order,
// reversed when the direction is `desc`.

export type MoveIntent
  = | { before: string | null }
  /** Descending, the gap is the screen's very bottom but not every row is loaded: the first saved member is unknown. */
    | { before: 'first-saved' }

/** Where a row dropped into `gap` lands in the list once it has left `from`. */
export function landingIndex(from: number, gap: number) {
  return gap > from
    ? gap - 1
    : gap
}

/** The intent of moving `screen[from]` to `gap`, or null when that changes nothing. */
export function moveIntent(screen: readonly string[], from: number, gap: number, direction: 'asc' | 'desc', complete: boolean): MoveIntent | null {
  if (gap === from || gap === from + 1)
    return null
  if (direction === 'asc') {
    return {
      before: gap >= screen.length
        ? null
        : screen[gap]!,
    }
  }
  if (gap === 0)
    return { before: null }
  if (gap >= screen.length && !complete)
    return { before: 'first-saved' }
  return { before: screen[gap - 1]! }
}
