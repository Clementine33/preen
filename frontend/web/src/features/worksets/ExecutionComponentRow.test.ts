import type { ExecutionComponent } from '@/lib/api/types'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ExecutionComponentRow from './ExecutionComponentRow.vue'

function component(overrides: Partial<ExecutionComponent> = {}): ExecutionComponent {
  return {
    component_index: 1,
    kept_decisions: [],
    component_id: 'comp-a',
    root_path: '/music/albumA',
    partition: 'matched',
    status: 'succeeded',
    operations: 1,
    completed_operations: 1,
    committed: ['/music/albumA/00.mp3'],
    removed: ['/music/albumA/00.m4a'],
    remaining: [],
    recovery: ['/music/albumA/Delete/00.m4a'],
    inventory_synced: true,
    ...overrides,
  }
}

describe('executionComponentRow', () => {
  it('keeps the partial facts of a failure: stage, error and unrun operations', () => {
    const wrapper = mount(ExecutionComponentRow, {
      props: {
        component: component({
          status: 'failed',
          stage: 'materialize',
          operations: 2,
          completed_operations: 0,
          committed: [],
          removed: [],
          remaining: ['encode:/music/albumA/00.wav'],
          error_code: 'COMPONENT_ENCODE_FAILED',
          error_message: 'ffmpeg failed',
          inventory_synced: false,
          inventory_sync_error: 'database is locked',
        }),
      },
    })

    expect(wrapper.text()).toContain('失败')
    expect(wrapper.text()).toContain('ffmpeg failed')
    expect(wrapper.text()).toContain('停在生成输出')
    expect(wrapper.get('[data-testid="execution-remaining"]').text()).toContain('00.wav')
    expect(wrapper.get('[data-testid="execution-inventory-warning"]').text()).toContain('未完全同步')
  })

  it('names the soft-delete destination and the hard-delete consequence', () => {
    expect(mount(ExecutionComponentRow, {
      props: {
        component: component(),
      },
    }).text()).toContain('已清理到 Delete/')
    expect(mount(ExecutionComponentRow, {
      props: {
        component: component(),
        hardDelete: true,
      },
    }).text()).toContain('已硬删除')
  })

  it('shows the plan\'s kept files, not where a run moved files', () => {
    const wrapper = mount(ExecutionComponentRow, {
      props: {
        component: component({
          kept_decisions: [
            { path: '/music/albumA/disc1/00.wav', resolution: 'keep', reason_code: 'UNMET_TARGET' },
            { path: '/music/albumA/01.mp3', resolution: 'keep', reason_code: 'KEEP_ENCODED_SATISFIED' },
          ],
        }),
      },
    })

    const kept = wrapper.get('[data-testid="execution-kept"]').text()
    expect(kept).toContain('disc1/00.wav')
    expect(kept).not.toContain('/music/albumA/')
    expect(wrapper.get('[title="/music/albumA/disc1/00.wav"]').text()).toBe('disc1/00.wav')
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    expect(wrapper.get('summary').text()).toContain('保留 2')
    expect(kept).toContain('目标未满足')
    expect(kept).toContain('已满足编码目标')
    expect(wrapper.find('[title="/music/albumA/Delete/00.m4a"]').exists()).toBe(false)
  })
})
