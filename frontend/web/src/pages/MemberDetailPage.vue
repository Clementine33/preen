<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMemberReview } from '@/composables/use-member-review'
import MemberReview from '@/features/worksets/MemberReview.vue'
import { useApiClient } from '@/lib/api/client'
import { CONVERSION } from '@/lib/api/types'
import { currentRecordQueryOptions } from '@/queries/worksets'

/**
 * One member's frozen review (`/worksets/:L/conversion/:M`). The page renders
 * inside whichever carrier the container chose; routing here never changes the
 * URL between carriers.
 */
const route = useRoute()
const router = useRouter()
const libraryId = computed(() => (route.params.libraryId as string) || '')
const api = useApiClient()
const recordQuery = useQuery(() => currentRecordQueryOptions(api, libraryId.value, CONVERSION))
const record = computed(() => recordQuery.data.value?.workset ?? null)
const worksetId = computed(() => record.value?.workset_id ?? null)

const memberId = computed(() => (route.params.memberId as string) || null)
const member = computed(() => record.value?.members.find(m => m.member_id === memberId.value) ?? null)
const { review, reviewQuery } = useMemberReview(worksetId, memberId)

function edit() {
  void router.push({
    name: 'conversion-member-edit',
    params: { libraryId: libraryId.value, memberId: memberId.value ?? '' },
  })
}
</script>

<template>
  <div>
    <p v-if="!member" class="p-3 text-xs text-[var(--danger-ink)]" role="alert" data-testid="member-not-found">
      未找到该文件夹：它可能不属于当前工作集。
    </p>
    <p v-else-if="reviewQuery.error.value" role="alert" class="p-3 text-xs text-[var(--danger-ink)]">成员计划读取失败。</p>
    <p v-else-if="!review" class="p-3 text-xs text-[var(--text-muted)]">加载中…</p>
    <div v-else class="min-h-0">
      <MemberReview :member="member" :review="review" :editable="true" @edit="edit" />
    </div>
  </div>
</template>
