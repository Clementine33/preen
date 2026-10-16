import { computed, type MaybeRefOrGetter } from 'vue'
import type { UseQueryReturnType } from '@tanstack/vue-query'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { useApiClient } from '@/lib/api/client'
import type { OperationType, TaskSettings, TaskSettingsMap } from '@/lib/api/types'
import { queryKeys } from '@/queries/query-keys'
import { saveTaskSettingsMutationOptions, taskSettingsQueryOptions } from '@/queries/worksets'
import { useSettingsForm } from './use-settings-form'

/**
 * The settings form of one Task kind: its settings read and written through
 * the Task settings endpoint (never a record's draft), so it works with or
 * without a record (ADR 0008). `toView` fills what the server omits; edits go
 * back over the stored object, so a field nobody touched is sent as stored.
 * The kind is fixed for the instance: mount one per kind.
 */
export function useTaskSettingsForm<T extends OperationType, V extends object>(options: {
  operation: T
  /** The stored settings as the form reads them (server-omitted fields filled in). */
  toView: (settings: TaskSettingsMap[T]) => V
  /** What a fresh install starts from; undefined while unknown. */
  defaults: MaybeRefOrGetter<V | undefined>
  /** Names the kind in refusals: 转换, 重命名, 元数据刷新. */
  label: string
}) {
  const api = useApiClient()
  const queryClient = useQueryClient()
  // The generic kind defeats Vue Query's inference; the helpers' own signatures hold the types.
  const query = useQuery(taskSettingsQueryOptions(api, options.operation) as never) as unknown as UseQueryReturnType<
    TaskSettings<T>,
    Error
  >
  const mutation = useMutation(saveTaskSettingsMutationOptions(api, queryClient, options.operation) as never) as unknown as {
    mutateAsync: (input: { settings: TaskSettingsMap[T]; etag: string }) => Promise<TaskSettings<T>>
  }
  const saved = computed(() => (query.data.value ? options.toView(query.data.value.settings) : undefined))

  const form = useSettingsForm<V>({
    saved,
    defaults: options.defaults,
    label: options.label,
    save: (edits) => {
      const current = query.data.value
      if (!current) return Promise.reject(new Error('设置尚未载入'))
      return mutation.mutateAsync({
        settings: { ...current.settings, ...edits } as TaskSettingsMap[T],
        etag: current.etag,
      })
    },
    onConflict: () =>
      void queryClient.invalidateQueries({ queryKey: queryKeys.taskSettings.detail(options.operation) }),
  })
  return { form, query, saved }
}
