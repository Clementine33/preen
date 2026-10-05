import { nextTick, onBeforeUnmount, watch, type Ref } from 'vue'
import { useRouter } from 'vue-router'

// Where the wall was when a work was opened from it. It belongs to that list
// entry in the browser history — not to the address, which says what is
// listed, and not to a store, which BACK knows nothing about — so returning to
// the entry finds it and any other way of reaching the wall does not.

export interface WallReturn {
  scrollTop: number
  focusRj: string
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
    router.options.history.replace(router.currentRoute.value.fullPath, { [KEY]: { ...state } })
  }
}

/**
 * Puts the wall back once the listing it belongs to is on screen, exactly
 * once. Back from a work: the saved scroll offset, then focus on the work that
 * was opened, without scrolling again. Any other arrival (reload, link) with a
 * selected work only scrolls that work into view; focus stays where it is.
 *
 * `ready` is true once settled data for the current address has rendered, so
 * a cold start or a slow listing simply restores later. Pointer or key input
 * before then cancels the focus: the user has moved on, and a late focus would
 * steal it.
 */
export function useWallRestore(options: {
  ready: () => boolean
  wall: Ref<WallHandle | null>
  selectedRj: () => string | null
}): void {
  const router = useRouter()
  const raw = router.options.history.state[KEY] as WallReturn | null | undefined
  const saved = raw && typeof raw.scrollTop === 'number' && typeof raw.focusRj === 'string' ? raw : null
  let done = false
  let focusCancelled = false

  const cancel = () => {
    focusCancelled = true
  }
  const listen = (add: boolean) => {
    for (const type of ['pointerdown', 'keydown'] as const) {
      if (add) document.addEventListener(type, cancel, { capture: true })
      else document.removeEventListener(type, cancel, { capture: true })
    }
  }
  listen(true)
  onBeforeUnmount(() => listen(false))

  watch(options.ready, async (ready) => {
    if (!ready || done) return
    done = true
    listen(false)
    await nextTick()
    const wall = options.wall.value
    if (!wall) return
    if (saved) {
      if (wall.scroller) wall.scroller.scrollTop = saved.scrollTop
      if (!focusCancelled) wall.cellFor(saved.focusRj)?.focus({ preventScroll: true })
      // Used once: a later visit to this entry by other means starts fresh.
      router.options.history.replace(router.currentRoute.value.fullPath, { [KEY]: null })
      return
    }
    const rj = options.selectedRj()
    if (rj) wall.cellFor(rj)?.scrollIntoView?.({ block: 'nearest' })
  }, { immediate: true, flush: 'post' })
}
