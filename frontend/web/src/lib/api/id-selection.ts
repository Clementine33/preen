/**
 * Request encoding for "these of those": a selection over a known list of ids
 * goes on the wire as the shorter of the two ways to say it, so the body does
 * not grow with the list (docs/api/worksets.md#creating-the-current-record).
 */

export interface IdSelection {
  /** `true`: every id of the universe but `ids`; `false`: exactly `ids`. */
  all: boolean
  ids: string[]
}

/**
 * Names `picked` among `universe`: the picked ids, or — when more than half are
 * picked — every id but the unpicked ones. Ids outside the universe are not
 * sent; ties keep the plain list.
 */
export function shorterIdSelection(picked: ReadonlySet<string>, universe: readonly string[]): IdSelection {
  const chosen: string[] = []
  const rest: string[] = []
  for (const id of universe) {
    (picked.has(id)
      ? chosen
      : rest).push(id)
  }
  return rest.length < chosen.length
    ? { all: true, ids: rest }
    : { all: false, ids: chosen }
}
