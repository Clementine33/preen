import type { ExecutionView, MemberSummary } from '@/lib/api/types'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ExecutionPanel from './ExecutionPanel.vue'

function view(overrides: Partial<ExecutionView> = {}): ExecutionView {
  return {
    execution_id: 'exec-1',
    workset_id: 'ws-1',
    operation_type: 'conversion',
    plan_id: 'plan-1',
    status: 'running',
    options: { delete_mode: 'soft' },
    total_components: 2,
    completed_components: 1,
    total_operations: 3,
    completed_operations: 1,
    current_root: '/music/albumB',
    current_component_id: 'comp-b',
    current_phase: 'component',
    error_code: '',
    error_message: '',
    started_at: '',
    finished_at: '',
    created_at: '',
    ...overrides,
  }
}

function summary(id: string, status: MemberSummary['status'], overrides: Partial<MemberSummary> = {}): MemberSummary {
  return {
    member_id: id,
    root_path: `/music/${id}`,
    status,
    total_components: 2,
    completed_components: 1,
    total_operations: 4,
    completed_operations: 2,
    error_message: '',
    inventory_sync_error: '',
    ...overrides,
  }
}

const counts = {
  succeeded: 7,
  failed: 1,
  canceled: 0,
  running: 1,
  pending: 3,
}

describe('executionPanel', () => {
  it('lists counts and only the exception members, each linking to its member page', () => {
    const wrapper = mount(ExecutionPanel, {
      props: {
        members: [
          {
            member_id: 'm-1',
            folder_path: '/music/m-1',
            folder_name: '专辑 A',
            rel_path: '专辑 A',
            dir_id: 'd-1',
          },
        ],
        memberRoute: (memberId: string) => ({
          name: 'x',
          params: { memberId },
        }),
        view: view({
          members: {
            counts,
            attention: [
              summary('m-1', 'failed', { error_message: 'ffmpeg failed', inventory_sync_error: 'database is locked' }),
              summary('m-2', 'running'),
            ],
            attention_total: 2,
          },
        }),
      },
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :data-to="JSON.stringify(to)"><slot /></a>',
          },
        },
      },
    })

    expect(wrapper.get('[data-testid="execution-count-succeeded"]').text()).toBe('7')
    expect(wrapper.get('[data-testid="execution-count-failed"]').text()).toBe('1')
    expect(wrapper.get('[data-testid="execution-count-pending"]').text()).toBe('3')
    const items = wrapper.findAll('[data-testid="execution-attention-item"]')
    expect(items).toHaveLength(2)
    expect(items[0]!.get('[data-testid="execution-member-name"]').text()).toBe('专辑 A')
    expect(items[0]!.get('[data-testid="execution-member-name"]').attributes('data-to')).toContain('"memberId":"m-1"')
    expect(items[0]!.get('[data-testid="execution-member-status"]').text()).toBe('失败')
    expect(items[0]!.text()).toContain('组件 1/2')
    expect(items[0]!.text()).toContain('ffmpeg failed')
    expect(items[0]!.get('[data-testid="execution-inventory-warning"]').text()).toContain('database is locked')
    // A member the record cannot name falls back to its path.
    expect(items[1]!.get('[data-testid="execution-member-name"]').text()).toBe('…/music/m-2')
    expect(wrapper.find('[data-testid="execution-attention-more"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="execution-component"]').exists()).toBe(false)
  })

  it('says how many exceptions the server did not list', () => {
    const wrapper = mount(ExecutionPanel, {
      props: {
        view: view({
          members: {
            counts,
            attention: Array.from({ length: 50 }, (_, i) => summary(`m-${i}`, 'failed')),
            attention_total: 63,
          },
        }),
      },
    })

    expect(wrapper.findAll('[data-testid="execution-attention-item"]')).toHaveLength(50)
    expect(wrapper.get('[data-testid="execution-attention-more"]').text()).toBe('另有 13 个')
  })

  it('explains a canceled run without claiming the whole revision happened', () => {
    const wrapper = mount(ExecutionPanel, {
      props: {
        view: view({ status: 'canceled' }),
      },
    })

    expect(wrapper.get('[data-testid="execution-status"]').text()).toBe('已取消')
    expect(wrapper.text()).toContain('已完成的组件结果保留')
  })

  it('says which folders a scoped run covered, and what is left for the next plan', () => {
    const members = [
      {
        member_id: 'm-1',
        folder_path: '/music/albumA',
        folder_name: 'albumA',
        rel_path: 'albumA',
        dir_id: 'd-1',
      },
      {
        member_id: 'm-2',
        folder_path: '/music/albumB',
        folder_name: 'albumB',
        rel_path: 'albumB',
        dir_id: 'd-2',
      },
    ]

    const scoped = mount(ExecutionPanel, {
      props: {
        members,
        view: view({
          status: 'succeeded',
          selected_folders: ['albumA'],
        }),
      },
    })
    const note = scoped.get('[data-testid="execution-scope"]').text()
    expect(note).toContain('本次只执行了选中的 1 个文件夹')
    expect(note).toContain('记录共 2 个')
    expect(note).toContain('需重新生成计划后执行')

    // A run that covered the record is not described as partial.
    const whole = mount(ExecutionPanel, {
      props: {
        members,
        view: view({
          status: 'succeeded',
          selected_folders: ['albumA', 'albumB'],
        }),
      },
    })
    expect(whole.find('[data-testid="execution-scope"]').exists()).toBe(false)

    // A session started before the scope existed carries no scope at all.
    const legacy = mount(ExecutionPanel, {
      props: {
        members,
        view: view({
          status: 'succeeded',
        }),
      },
    })
    expect(legacy.find('[data-testid="execution-scope"]').exists()).toBe(false)
  })

  it('states a session-level failure in the user\'s words', () => {
    const wrapper = mount(ExecutionPanel, {
      props: {
        view: view({
          status: 'failed',
          error_code: 'INPUT_CHANGED',
          error_message: 'the scanned inventory no longer matches the revision\'s recorded inputs; regenerate the plan',
        }),
      },
    })

    expect(wrapper.get('[data-testid="execution-error"]').text()).toContain('文件夹输入已变化')
    expect(wrapper.get('[data-testid="execution-error"]').text()).not.toContain('regenerate the plan')
  })

  it('reports real counts while running, never a fabricated percentage', () => {
    const wrapper = mount(ExecutionPanel, {
      props: {
        view: view(),
      },
    })

    expect(wrapper.get('[data-testid="execution-status"]').text()).toBe('执行中')
    expect(wrapper.get('[data-testid="execution-progress"]').text()).toContain('组件 1/2')
    expect(wrapper.get('[data-testid="execution-progress"]').text()).toContain('操作 1/3')
    expect(wrapper.text()).toContain('albumB')
  })
})
