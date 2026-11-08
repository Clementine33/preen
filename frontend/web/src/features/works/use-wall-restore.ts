import type { Ref } from 'vue'
import { nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'

// Where the wall was when it was left, by any route (a work opened, the player
// bar, a nav link). It belongs to that list entry in the browser history — not
// to the address, which says what is listed, and not to a store, which BACK
// knows nothing about — so returning to the entry finds it and any other way
// of reaching the wall does not. It is restored once, on BACK, to exactly that
// offset; the selected work takes focus when nothing else holds it. Without
// one, the selected work is only brought into view.

export interface WallReturn {
  scrollTop: number
}

/** What the restore needs from the wall: its scroller and one cell. */
export interface WallHandle {
  scroller: HTMLElement | null
  cellFor: (rj: string) => HTMLElement | null
}

const KEY = 'wallReturn'

/**
 * Records the wall's position on the current history entry, just before the
 * page navigates away from it. Writing the entry's state is not a navigation.
 */
export function useRecordWallReturn() {
  const router = useRouter()
  return (state: WallReturn) => {
    const { currentRoute } = router
    const { history } = router.options
    // Leaving by BACK/FORWARD has already moved the history to the destination:
    // writing now would overwrite that entry's address with this one.
    if (history.location !== currentRoute.value.fullPath)
      return
    history.replace(currentRoute.value.fullPath, {
      [KEY]: { ...state },
    })
  }
}

/**
 * Puts the wall back once the listing it belongs to is on screen, exactly
 * once. Back with a saved position: exactly the saved scroll offset, then
 * focus on the selected work (without scrolling) if nothing else holds focus.
 * Any other arrival (reload, link) with a selected work only scrolls that work
 * into view; focus stays where it is.
 *
 * `ready` is true once settled data for the current address has rendered, so
 * a cold start or a slow listing simply restores later.
 */
export function useWallRestore(options: {
  ready: () => boolean
  wall: Ref<WallHandle | null>
  selectedRj: () => string | null
}): void {
  const router = useRouter()
  const raw = router.options.history.state[KEY] as WallReturn | null | undefined
  const saved = raw && typeof raw.scrollTop === 'number'
    ? raw
    : null
  let done = false

  watch(options.ready, async (ready) => {
    if (!ready || done)
      return
    done = true
    await nextTick()
    const wall = options.wall.value
    if (!wall)
      return
    const rj = options.selectedRj()
    const cell = rj
      ? wall.cellFor(rj)
      : null
    // The saved offset is the user's own place, even when they had scrolled
    // away from the selection; only an arrival without one is placed by it.
    if (!saved) {
      // Next frame: the wall has measured its columns by then.
      requestAnimationFrame(() => cell?.scrollIntoView?.({ block: 'nearest' }))
      return
    }
    if (wall.scroller)
      wall.scroller.scrollTop = saved.scrollTop
    const active = document.activeElement
    if (!active || active === document.body)
      cell?.focus({ preventScroll: true })
    // Used once: a later visit to this entry by other means starts fresh.
    router.options.history.replace(router.currentRoute.value.fullPath, { [KEY]: null })
  }, { immediate: true, flush: 'post' })
}
