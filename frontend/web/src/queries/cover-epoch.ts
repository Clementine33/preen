import { ref } from 'vue'

/**
 * Counts successful list/detail reads. A local cover that failed to load is
 * retried once a later read lands, even when its address did not change: the
 * file may be readable again (a share back online) with no new version to show.
 */
export const coverEpoch = ref(0)

export function bumpCoverEpoch(): void {
  coverEpoch.value += 1
}
