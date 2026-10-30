import type { VueWrapper } from '@vue/test-utils'
import type { ComponentOutcome, MemberReviewResponse, Workset } from '@/lib/api/types'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { apiClientKey } from '@/lib/api/client'
import { apiStub } from '@/test/api-stub'
import { frozenConclusion } from '@/test/member-conclusion'
import { installTestQueryPlugin } from '@/test/query-client'
import PlanReviewTree from './PlanReviewTree.vue'

/**
 * The plan's own view of a member (ADR 0001).
 *
 * It renders the frozen conclusions — kept, deleted, generated — and marks a
 * planned output as pending because that file does not exist yet. It reads no
 * directory listing: a plan review built from the latest scan would be a
 * different promise than the one the plan makes.
 */

const workset: Workset = {
  workset_id: 'ws-1',
  title: 'Archive',
  version: 1,
  library: { library_id: 'lib-1', name: 'Archive', root_path: '/music' },
  members: [
    {
      member_id: 'm-1',
      folder_path: '/music/albumA',
      folder_name: 'albumA',
      rel_path: 'albumA',
      dir_id: 'dir-albumA',
    },
  ],
  operations: [],
  updated_at: '',
  created_at: '',
}

function component(): ComponentOutcome {
  return {
    component_id: 'c-1',
    partition: 'matched',
    status: 'ok',
    lanes: [],
    variant_decisions: [
      {
        stem: 'track1',
        decisions: [
          // Real codes: a mapped keep reason, and a delete reason the review is
          // deliberately silent about.
          { path: '/music/albumA/keep.flac', resolution: 'keep', reason_code: 'KEEP_ENCODED_SATISFIED' },
          { path: '/music/albumA/obsolete.mp3', resolution: 'delete', reason_code: 'OBSOLETE_ENCODED' },
          { path: '/music/albumA/track1.wav', resolution: 'encode', target_path: '/music/albumA/track1.wav' },
        ],
      },
    ],
    operations: [
      {
        kind: 'encode',
        phase: 'execute',
        component_id: 'c-1',
        variant_stem: 'track1',
        source_path: '/music/albumA/track1.flac',
        target_path: '/music/albumA/track1.wav',
      },
    ],
    projected_inventory: ['/music/albumA/track1.wav'],
    files: [],
  }
}

const changedConclusion = frozenConclusion('changed')
changedConclusion.task!.matched.changes = true

const revision: MemberReviewResponse = {
  plan_id: 'plan-1',
  task: {
    kind: 'conversion',
    schema_version: 1,
    payload: {
      policy: {},
      policy_hash: 'ph',
      classifier: {},
      summary: {
        component_count: 1,
        blocked_count: 0,
        operation_count: 1,
        error_count: 0,
        summary_reason: 'ACTIONABLE',
      },
      components: [
        component(),
      ],
    },
  },
  member_id: 'm-1',
  member_name: 'albumA',
  folder_path: '/music/albumA',
  excluded: false,
  effective: { schema_version: 1 },
  sources: {},
  conclusion: changedConclusion,
  roots: [
    {
      root_index: 0,
      root_path: '/music/albumA',
      root_status: 'ok',
      root_error_code: '',
      root_error_message: '',
      stale: false,
      inventory_fingerprint: 'fp',
      entry_count: 3,
    },
  ],

}

/**
 * A plan whose files sit two directories deep, plus a projected output inside
 *  the deepest one: a member is a folder shape, not a flat list.
 */
function nestedRevision(): Partial<MemberReviewResponse> {
  return {
    task: {
      kind: 'conversion',
      schema_version: 1,
      payload: {
        policy: {},
        policy_hash: 'ph',
        classifier: {},
        summary: {
          component_count: 1,
          blocked_count: 0,
          operation_count: 1,
          error_count: 0,
          summary_reason: 'ACTIONABLE',
        },
        components: [
          {
            ...component(),
            variant_decisions: [
              {
                stem: 'track1',
                decisions: [
                  { path: '/music/albumA/keep.flac', resolution: 'keep', reason_code: 'KEEP_ENCODED_SATISFIED' },
                  { path: '/music/albumA/sub/nested.mp3', resolution: 'delete', reason_code: 'OBSOLETE_ENCODED' },
                  {
                    path: '/music/albumA/sub/deep/track1.wav',
                    resolution: 'encode',
                    target_path: '/music/albumA/sub/deep/track1.wav',
                  },
                ],
              },
            ],
            projected_inventory: ['/music/albumA/sub/deep/track2.wav'],
          },
        ],
      },
    },
  }
}

async function mountTree(
  overrides: {
    memberId?: string
    revision?: Partial<MemberReviewResponse>
    operation?: Record<string, unknown>
  } = {},
) {
  const api = apiStub({
    getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
    getWorkset: vi.fn().mockResolvedValue(workset),
    getOperation: vi.fn().mockResolvedValue(
      overrides.operation ?? {
        workset_id: 'ws-1',
        operation_type: 'conversion',
        version: 2,
        planning_state: 'planned',
        current_revision: { plan_id: 'plan-1', revision_index: 1, validation_state: 'valid' },
        active_generation: null,
        latest_generation: null,
        active_execution: null,
        latest_execution: null,
      },
    ),
    getOperationDraft: vi.fn().mockResolvedValue({
      version: 2,
      document: { members: [] },
    }),
    getMemberReview: vi.fn().mockResolvedValue({
      ...revision,
      ...(overrides.memberId === 'm-2'
        ? {
            member_id: 'm-2',
            folder_path: '/music/albumB',
            roots: [],
            conclusion: {
              member_id: 'm-2',
              excluded: false,
              input: 'none',
              has_components: false,
            },
            task: {
              ...revision.task,
              payload: { ...revision.task.payload, components: [] },
            },
          }
        : {}),
      ...overrides.revision,
    }),
  } as never)
  const wrapper = mount(PlanReviewTree, {
    props: { worksetId: 'ws-1', memberId: overrides.memberId ?? 'm-1' },
    global: {
      plugins: [
        createPinia(),
        installTestQueryPlugin(),
        createRouter({
          history: createMemoryHistory(),
          routes: [
            {
              path: '/',
              component: { template: '<div />' },
            },
          ],
        }),
      ],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  expect(api.getMemberReview).toHaveBeenCalledExactlyOnceWith('ws-1', 'conversion', 'plan-1', overrides.memberId ?? 'm-1', expect.any(AbortSignal))
  expect(api.getRevision).not.toHaveBeenCalled()
  expect(api.getMemberTree).not.toHaveBeenCalled()
  return wrapper
}

beforeEach(() => {
  localStorage.clear()
})

describe('plan review tree', () => {
  it('renders the frozen decisions of the plan', async () => {
    const wrapper = await mountTree()

    const rows = wrapper.findAll('[role="treeitem"] li, [data-resolution]')
    const text = wrapper.text()
    expect(text).toContain('keep.flac')
    expect(text).toContain('保留')
    expect(text).toContain('obsolete.mp3')
    expect(text).toContain('删除')
    expect(text).toContain('track1.wav')
    expect(text).toContain('生成')
    expect(rows.length).toBeGreaterThan(0)

    // The plan's code is read with the member review's own rule: a kept file
    // says why in the plan's words, and a code the map does not cover is never
    // spilled into the tree as SCREAMING_SNAKE.
    expect(text).toContain('已满足编码目标')
    expect(text).not.toContain('KEEP_ENCODED_SATISFIED')
    expect(text).not.toContain('OBSOLETE_ENCODED')
  })

  it('marks a planned output as pending, because it is not on disk yet', async () => {
    const wrapper = await mountTree()

    const pending = wrapper.findAll('[data-pending="true"]')
    expect(pending.length).toBe(1)
    expect(pending[0].text()).toContain('track1.wav')
    expect(pending[0].text()).toContain('待生成')
  })

  it('offers no way to manage files: the plan review is read-only', async () => {
    const wrapper = await mountTree()

    expect(wrapper.find('[data-testid="file-toolbar"]').exists()).toBe(false)
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="row-actions"]').exists()).toBe(false)
  })

  it('summarises what the plan does with this folder before the rows', async () => {
    const wrapper = await mountTree()

    // The same reader the conversion list's row uses, so the row that opened
    // this page and the page itself never disagree.
    const summary = wrapper.get('[data-testid="plan-summary"]').text()
    expect(summary).toContain('有变化')
    expect(summary).toContain('无音效：将转换')

    const tally = wrapper.get('[data-testid="plan-tally"]').text()
    expect(tally).toContain('保留 1')
    expect(tally).toContain('删除 1')
    expect(tally).toContain('生成 1')
    expect(tally).toContain('1 项待生成')
  })

  it('says why a folder the plan does not cover has nothing to list', async () => {
    const wrapper = await mountTree({ memberId: 'm-2' })

    expect(wrapper.get('[data-testid="plan-summary"]').text()).toContain('未参与')
    expect(wrapper.get('[data-testid="plan-empty"]').text()).toContain('没有列出这个文件夹的文件')
    expect(wrapper.find('[data-pending="true"]').exists()).toBe(false)
  })

  it('keeps the frozen plan on screen and says it needs regenerating', async () => {
    const wrapper = await mountTree({
      operation: {
        workset_id: 'ws-1',
        operation_type: 'conversion',
        version: 3,
        planning_state: 'needs_planning',
        current_revision: { plan_id: 'plan-1', revision_index: 1, validation_state: 'stale' },
        active_generation: null,
        latest_generation: null,
        active_execution: null,
        latest_execution: null,
      },
    })

    expect(wrapper.get('[data-testid="plan-needs-regeneration"]').text()).toContain('需重新生成')
    // The proposal itself is untouched: a stale plan is still the plan.
    expect(wrapper.get('[data-testid="plan-tally"]').text()).toContain('保留 1')
  })

  it('says so when the record has no plan yet', async () => {
    const api = apiStub({
      getCurrentRecord: vi.fn().mockResolvedValue({ workset }),
      getWorkset: vi.fn().mockResolvedValue(workset),
      getOperation: vi.fn().mockResolvedValue({
        workset_id: 'ws-1',
        operation_type: 'conversion',
        version: 1,
        planning_state: 'unplanned',
        current_revision: null,
        active_generation: null,
        latest_generation: null,
        active_execution: null,
        latest_execution: null,
      }),
      getOperationDraft: vi.fn().mockResolvedValue({
        version: 1,
        document: { members: [] },
      }),
    } as never)
    const wrapper = mount(PlanReviewTree, {
      props: { worksetId: 'ws-1', memberId: 'm-1' },
      global: {
        plugins: [
          createPinia(),
          installTestQueryPlugin(),
          createRouter({
            history: createMemoryHistory(),
            routes: [
              {
                path: '/',
                component: { template: '<div />' },
              },
            ],
          }),
        ],
        provide: { [apiClientKey as symbol]: api },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('还没有当前计划')
  })
})

describe('plan review tree: nested directories', () => {
  function treeText(wrapper: VueWrapper): string {
    return wrapper.get('[role="tree"]').text()
  }

  it('shows every level of the member, not just the first', async () => {
    const wrapper = await mountTree({
      revision: nestedRevision(),
    })

    const text = treeText(wrapper)
    for (const name of [
      'sub',
      'deep',
      'nested.mp3',
      'track1.wav',
      'track2.wav',
    ]) {
      expect(text).toContain(name)
    }
    // Depth-first, and a directory's own files come before its children.
    expect(text.indexOf('sub')).toBeLessThan(text.indexOf('nested.mp3'))
    expect(text.indexOf('nested.mp3')).toBeLessThan(text.indexOf('deep'))
    expect(text.indexOf('deep')).toBeLessThan(text.indexOf('track1.wav'))

    // Both levels are open by default.
    expect(wrapper.find('button[aria-label="折叠 sub"]').exists()).toBe(true)
    expect(wrapper.find('button[aria-label="折叠 deep"]').exists()).toBe(true)

    // A projected output two levels down is still marked as not on disk yet.
    const pending = wrapper.findAll('[data-pending="true"]')
    expect(pending.length).toBe(2)
    expect(pending[0].text()).toContain('track1.wav')
    expect(pending[1].text()).toContain('track2.wav')
  })

  it('hides a collapsed directory and everything under it, and brings it back', async () => {
    const wrapper = await mountTree({
      revision: nestedRevision(),
    })

    await wrapper.get('button[aria-label="折叠 sub"]').trigger('click')
    expect(treeText(wrapper)).not.toContain('nested.mp3')
    expect(treeText(wrapper)).not.toContain('deep')
    expect(treeText(wrapper)).not.toContain('track1.wav')
    // The summary still counts the whole plan, collapsed or not.
    expect(wrapper.get('[data-testid="plan-tally"]').text()).toContain('保留 1')
    expect(wrapper.get('[data-testid="plan-tally"]').text()).toContain('删除 1')
    expect(wrapper.get('[data-testid="plan-tally"]').text()).toContain('生成 2')

    await wrapper.get('button[aria-label="展开 sub"]').trigger('click')
    expect(treeText(wrapper)).toContain('deep')
    expect(treeText(wrapper)).toContain('track1.wav')
    expect(treeText(wrapper)).toContain('track2.wav')
  })

  it('collapses one level without touching its siblings', async () => {
    const wrapper = await mountTree({
      revision: nestedRevision(),
    })

    await wrapper.get('button[aria-label="折叠 deep"]').trigger('click')
    expect(treeText(wrapper)).not.toContain('track1.wav')
    // The collapsed row itself stays: it is how the level is opened again.
    expect(treeText(wrapper)).toContain('deep')
    expect(wrapper.find('button[aria-label="展开 deep"]').exists()).toBe(true)
    // The parent keeps its own file, and stays open.
    expect(treeText(wrapper)).toContain('nested.mp3')
    expect(wrapper.find('button[aria-label="折叠 sub"]').exists()).toBe(true)
  })
})
