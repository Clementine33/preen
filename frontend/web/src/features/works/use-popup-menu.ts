import { nextTick, onBeforeUnmount, onMounted, type Ref } from 'vue'

/**
 * The shared behaviour of the wall's popup menus (the library switcher, the
 * filter rail's selects): open focuses the current option — or the search box
 * when the menu has one — ↑↓ and J/K move between options, Esc closes back to
 * the trigger, and a pointer or focus leaving the menu closes it.
 *
 * The options are read from the DOM when a key needs them, never cached: the
 * list changes while the menu lives (libraries load, a search narrows it).
 * Handled keys stop here, and the panel carries `data-own-keys`, so the wall's
 * keyboard never acts underneath an open menu.
 *
 * A virtualized list only has the rows in view in the DOM, so it passes `step`
 * (move the focus by one option, by index) and `focusCurrent` (focus the
 * selected option, scrolling it in first) instead of being read from the DOM.
 */
export function usePopupMenu(options: {
  root: Ref<HTMLElement | null>
  trigger: Ref<HTMLElement | null>
  open: Ref<boolean>
  step?: (delta: 1 | -1) => void
  focusCurrent?: () => void
}) {
  const { root, trigger, open } = options

  function items(): HTMLElement[] {
    return Array.from(root.value?.querySelectorAll<HTMLElement>('[role="option"]') ?? [])
  }

  function setOpen(value: boolean): void {
    open.value = value
    if (!value) return
    void nextTick(() => {
      const search = root.value?.querySelector<HTMLInputElement>('input[type="search"]')
      if (search) {
        search.focus({ preventScroll: true })
        return
      }
      if (options.focusCurrent) {
        options.focusCurrent()
        return
      }
      const list = items()
      const current = list.findIndex((item) => item.getAttribute('aria-selected') === 'true')
      list[Math.max(0, current)]?.focus({ preventScroll: true })
    })
  }

  function close(returnFocus: boolean): void {
    setOpen(false)
    if (returnFocus) trigger.value?.focus()
  }

  function onKeydown(event: KeyboardEvent): void {
    if (!open.value) return
    const list = items()
    const at = list.indexOf(document.activeElement as HTMLElement)
    const inSearch = event.target instanceof HTMLInputElement
    const down = event.key === 'ArrowDown' || (!inSearch && event.key === 'j')
    const up = event.key === 'ArrowUp' || (!inSearch && event.key === 'k')
    if (event.key === 'Escape') {
      close(true)
    } else if ((down || up) && options.step) {
      options.step(down ? 1 : -1)
    } else if (down) {
      list[Math.min(list.length - 1, at + 1)]?.focus({ preventScroll: true })
    } else if (up) {
      list[Math.max(0, at - 1)]?.focus({ preventScroll: true })
    } else {
      return
    }
    event.preventDefault()
    event.stopPropagation()
  }

  function onPointerDown(event: PointerEvent): void {
    if (open.value && root.value !== null && !root.value.contains(event.target as Node)) {
      setOpen(false)
    }
  }

  /** Tabbing out of the menu closes it; moving focus inside it does not. */
  function onFocusOut(event: FocusEvent): void {
    const next = event.relatedTarget as Node | null
    if (open.value && next !== null && root.value !== null && !root.value.contains(next)) {
      setOpen(false)
    }
  }

  onMounted(() => document.addEventListener('pointerdown', onPointerDown))
  onBeforeUnmount(() => document.removeEventListener('pointerdown', onPointerDown))

  return { setOpen, close, onKeydown, onFocusOut }
}
