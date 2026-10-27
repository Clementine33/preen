import type { MediaFactsStatus } from '@/lib/api/types'
import { defineStore } from 'pinia'

/**
 * The Media Facts Pass's latest reported status, owned by the root
 * `syncMediaFacts` connection. `null` until a status arrives and again after a
 * disconnect, so a page never shows a state the connection can no longer vouch for.
 */
export const useMediaFactsStore = defineStore('media-facts', {
  state: () => ({ status: null as MediaFactsStatus | null }),
})
