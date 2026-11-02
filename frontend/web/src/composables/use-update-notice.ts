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
  // Best-effort: blocked storage only means the toast can show again next visit.
  try {
    localStorage.setItem(UPDATE_NOTIFIED_KEY, version)
  }
  catch {
    /* storage unavailable */
  }
}

/**
 * Tells the user once per release that a newer one exists. The version is
 * recorded in this browser's localStorage as soon as the toast is shown, even
 * if another toast replaces it, so a release is announced a single time; the
 * About page keeps showing it. Called from the always-mounted AppShell.
 */
export function useUpdateNotice(): void {
  const { show } = useToast()
  const check = useQuery(updateCheckQueryOptions(useApiClient()))
  watch(check.data, (data) => {
    if (data?.state !== 'available' || readNotified() === data.latest_version)
      return
    recordNotified(data.latest_version)
    show({
      text: `发现新版本 ${data.latest_version}`,
      action: {
        label: '查看',
        run: () => { window.open(data.release_url, '_blank', 'noopener,noreferrer') },
      },
    })
  }, { immediate: true })
}
