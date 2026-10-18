import { computed, effectScope, ref, watch, type ComputedRef, type Ref } from 'vue'
import { REDUCED_MOTION_QUERY, useMediaQuery } from './use-media-query'

/**
 * The motion preference of this browser: follow the system, or reduce. It can
 * only add reduction — a system that asks for reduced motion always wins — and
 * it lives in localStorage alone, so each device keeps its own.
 *
 * One effective state serves CSS and JS: `<html>.reduce-motion` mirrors it for
 * stylesheets (the inline script in index.html sets it before first paint),
 * and `useReducedMotion()` hands script-driven motion the same value.
 */
export type MotionPreference = 'system' | 'reduce'

export const MOTION_STORAGE_KEY = 'preen-motion'
export const REDUCE_MOTION_CLASS = 'reduce-motion'

export const MOTION_OPTIONS: readonly { value: MotionPreference; label: string }[] = [
  { value: 'system', label: '跟随系统' },
  { value: 'reduce', label: '减少动效' },
]

function readStored(): MotionPreference {
  try {
    return localStorage.getItem(MOTION_STORAGE_KEY) === 'reduce' ? 'reduce' : 'system'
  } catch {
    return 'system'
  }
}

let state: { preference: Ref<MotionPreference>; systemReduces: Ref<boolean>; reduced: ComputedRef<boolean> } | null = null

export function useMotion() {
  // A detached scope: the first caller is a component, and its unmount must
  // not stop the watchers every later caller relies on.
  state ??= effectScope(true).run(() => {
    const preference = ref(readStored())
    const media = typeof window.matchMedia === 'function' ? window.matchMedia(REDUCED_MOTION_QUERY) : null
    const systemReduces = ref(media?.matches ?? false)
    media?.addEventListener('change', () => {
      systemReduces.value = media.matches
    })
    const reduced = computed(() => systemReduces.value || preference.value === 'reduce')
    watch(preference, (value) => {
      // Best-effort: blocked storage still keeps the choice for this session.
      try {
        localStorage.setItem(MOTION_STORAGE_KEY, value)
      } catch {
        /* storage unavailable */
      }
    })
    watch(reduced, (value) => document.documentElement.classList.toggle(REDUCE_MOTION_CLASS, value), { immediate: true })
    return { preference, systemReduces, reduced }
  })!
  return state
}

/**
 * The effective reduced-motion state, for motion a component's script drives.
 * The system half is read live from the component's own mount, like every
 * other media query here.
 */
export function useReducedMotion(): ComputedRef<boolean> {
  const system = useMediaQuery(REDUCED_MOTION_QUERY)
  const { preference } = useMotion()
  return computed(() => system.value || preference.value === 'reduce')
}
