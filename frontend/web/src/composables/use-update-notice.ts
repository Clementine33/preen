import { useQuery } from '@tanstack/vue-query'
import { watch } from 'vue'
import { useToast } from '@/composables/use-toast'
import { useApiClient } from '@/lib/api/client'
import { updateCheckQueryOptions } from '@/queries/update-check'

export const UPDATE_NOTIFIED_KEY = 'preen.update-notified'

function readNotified(): string | null {
  try {
    return localStorage.getItem(UPDATE_NOTIFIED_KEY)
  }
  catch {
    return null
  }
}

function recordNotified(version: string): void {
  // Best-effort: blocked storage only means the toast can show again next launch.
  try {
    localStorage.setItem(UPDATE_NOTIFIED_KEY, version)
  }
  catch {
    /* storage unavailable */
  }
}

/**
 * Tells the user that a newer release exists, on every launch until they open
 * it. The version is recorded in this browser's localStorage only when the
 * toast's action is taken, so a toast that expired or was replaced unseen shows
 * again next launch; the About page keeps showing the release. Called from the
 * always-mounted AppShell.
 */
export function useUpdateNotice(): void {
  const { show } = useToast()
  const check = useQuery(updateCheckQueryOptions(useApiClient()))
  watch(check.data, (data) => {
    if (data?.state !== 'available' || readNotified() === data.latest_version)
      return
    show({
      text: `发现新版本 ${data.latest_version}`,
      action: {
        label: '查看',
        run: () => {
          recordNotified(data.latest_version)
          window.open(data.release_url, '_blank', 'noopener,noreferrer')
        },
      },
    })
  }, { immediate: true })
}
