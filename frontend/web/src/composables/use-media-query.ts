import { onBeforeUnmount, onMounted, ref } from 'vue'

/** The wall's one layout breakpoint: at or below it, taps route instead of inspect. */
export const WALL_MOBILE_QUERY = '(max-width: 768px)'
export const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

/**
 * Whether a media query matches, kept live. It is read at mount, not at import,
 * so it reflects the media state when the component actually appears.
 */
export function useMediaQuery(query: string) {
  const matches = ref(false)
  let list: MediaQueryList | null = null
  const sync = () => {
    matches.value = list?.matches ?? false
  }
  onMounted(() => {
    list = typeof matchMedia === 'function'
      ? matchMedia(query)
      : null
    sync()
    list?.addEventListener('change', sync)
  })
  onBeforeUnmount(() => list?.removeEventListener('change', sync))
  return matches
}
