import type { UserPreferences } from '@/lib/api/types'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import { useToast } from '@/composables/use-toast'
import { useApiClient } from '@/lib/api/client'
import { queryKeys } from './query-keys'

/** The server-synced preferences (docs/api/preferences.md); empty until loaded. */
export function usePreferences() {
  const api = useApiClient()
  const query = useQuery({
    queryKey: queryKeys.preferences.detail(),
    queryFn: ({ signal }: { signal?: AbortSignal }) => api.getPreferences(signal),
  })
  /** The Format Preference, a stable empty list until the server answers. */
  const trackFormats = computed<readonly string[]>(() => query.data.value?.track_formats ?? [])
  return { query, trackFormats }
}

/**
 * Replaces the Format Preference. The cache follows the click at once; a
 * failed write puts the previous list back and says so. The page only sends
 * lists the server would not change, so a success needs no further sync.
 */
export function useSetTrackFormats() {
  const api = useApiClient()
  const client = useQueryClient()
  const toast = useToast()
  const key = queryKeys.preferences.detail()
  return useMutation({
    mutationFn: (trackFormats: string[]) => api.putPreferences({ track_formats: trackFormats }),
    onMutate: async (trackFormats) => {
      await client.cancelQueries({ queryKey: key })
      const previous = client.getQueryData<UserPreferences>(key)
      client.setQueryData<UserPreferences>(key, { track_formats: trackFormats })
      return { previous }
    },
    onError: (_error, _input, context) => {
      client.setQueryData(key, context?.previous)
      toast.show({ text: '格式偏好保存失败，已恢复原设置' })
    },
  })
}
