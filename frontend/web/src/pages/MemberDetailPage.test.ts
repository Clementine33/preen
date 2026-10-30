import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey, ApiError } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { frozenConclusion } from '@/test/member-conclusion'
import { installTestQueryPlugin } from '@/test/query-client'
import MemberDetailPage from './MemberDetailPage.vue'

const member = {
  member_id: 'm1',
  folder_path: '/music/a',
  folder_name: 'a',
  rel_path: 'a',
  dir_id: 'd1',
}
const review = {
  plan_id: 'p2',
  ...member,
  member_name: 'a',
  excluded: false,
  effective: { schema_version: 1, mode: 'strict' },
  sources: { mode: 'member' },
  conclusion: frozenConclusion('unchanged'),
  roots: [],
  task: {
    kind: 'conversion',
    schema_version: 1,
    payload: { components: [] },
  },
}

async function page(replaced = false) {
  const getMemberReview = vi.fn()
  if (replaced)
    getMemberReview.mockRejectedValueOnce(new ApiError(404, 'REVISION_NOT_FOUND', 'revision not found'))
  getMemberReview.mockResolvedValue(review)
  const getOperation = vi.fn().mockResolvedValue({
    current_revision: { plan_id: 'p2' },
  })
  if (replaced) {
    getOperation.mockResolvedValueOnce({
      current_revision: { plan_id: 'p1' },
    })
  }
  const api = apiStub({
    getCurrentRecord: vi.fn().mockResolvedValue({
      workset: {
        workset_id: 'ws',
        members: [member],
      },
    }),
    getOperation,
    getMemberReview,
  } as never)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/:libraryId/:memberId', component: MemberDetailPage },
    ],
  })
  await router.push('/lib/m1')
  await router.isReady()
  const wrapper = mount(MemberDetailPage, {
    global: {
      plugins: [
        createPinia(),
        router,
        installTestQueryPlugin(),
      ],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, api }
}

describe('member detail reads', () => {
  it('requests only the reviewed member and displays its frozen provenance', async () => {
    const { wrapper, api } = await page()
    expect(api.getMemberReview).toHaveBeenCalledExactlyOnceWith('ws', 'conversion', 'p2', 'm1', expect.any(AbortSignal))
    expect(api.getRevision).not.toHaveBeenCalled()
    expect(api.getOperationDraft).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('独立设置')
    expect(wrapper.find('[data-testid="member-review"]').exists()).toBe(true)
  })

  it('refreshes the operation and follows the replacement plan', async () => {
    const { wrapper, api } = await page(true)
    expect(api.getOperation).toHaveBeenCalledTimes(2)
    expect(api.getMemberReview).toHaveBeenNthCalledWith(1, 'ws', 'conversion', 'p1', 'm1', expect.any(AbortSignal))
    expect(api.getMemberReview).toHaveBeenNthCalledWith(2, 'ws', 'conversion', 'p2', 'm1', expect.any(AbortSignal))
    expect(api.getRevision).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="member-review"]').exists()).toBe(true)
  })
})
