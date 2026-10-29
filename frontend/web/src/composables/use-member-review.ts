import type { Ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import { useApiClient } from '@/lib/api/client'
import { CONVERSION } from '@/lib/api/types'
import { operationMemberReviewQueryOptions, operationQueryOptions } from '@/queries/worksets'

/** Reads one member's frozen plan without subscribing to the full revision. */
export function useMemberReview(worksetId: Ref<string | null>, memberId: Ref<string | null>) {
  const api = useApiClient()
  const queryClient = useQueryClient()
  const operationQuery = useQuery(() => operationQueryOptions(api, worksetId.value, CONVERSION))
  const operation = computed(() => operationQuery.data.value ?? null)
  const planId = computed(() => operation.value?.current_revision?.plan_id ?? null)
  const reviewQuery = useQuery(() => operationMemberReviewQueryOptions(
    api,
    queryClient,
    worksetId.value,
    CONVERSION,
    planId.value,
    memberId.value,
  ))
  const review = computed(() => reviewQuery.data.value ?? null)
  return { operation, review, reviewQuery }
}
