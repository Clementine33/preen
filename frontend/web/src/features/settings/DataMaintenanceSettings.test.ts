import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiClientKey } from '@/lib/api/client'
import type { CleanupCategory, CleanupPreview, CleanupRun } from '@/lib/api/types'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import DataMaintenanceSettings from './DataMaintenanceSettings.vue'

enableAutoUnmount(afterEach)

const ALL: CleanupCategory[] = [
  'scan_sessions', 'plan_generations', 'cover_downloads', 'works',
  'relation_reports', 'source_tags', 'source_voice_actors', 'source_circles',
]
const preview = (candidates: Partial<Record<CleanupCategory, number>>, categories = ALL): CleanupPreview => ({
  checked_at: '2026-10-23T04:00:00Z', categories, candidates,
  rules: {},
})
const run = (over: Partial<CleanupRun> = {}): CleanupRun => ({
  run_id: 'r1', categories: ['scan_sessions'], status: 'running', phase: 'scan_sessions',
  started_at: '2026-10-23T04:00:00Z', finished_at: null,
  counts: { scan_sessions: { checked: 5, deleted: 4, skipped: 1 } }, reason: '', ...over,
})

async function mountIt(api: ReturnType<typeof apiStub>) {
  const wrapper = mount(DataMaintenanceSettings, {
    attachTo: document.body.appendChild(document.createElement('div')),
    global: { plugins: [installTestQueryPlugin()], provide: { [apiClientKey as symbol]: api } },
  })
  await flushPromises()
  return wrapper
}
const tid = (id: string) => `[data-testid="${id}"]`

describe('data maintenance', () => {
  it('states only the automatic retention periods and schedule', async () => {
    const wrapper = await mountIt(apiStub())
    expect(wrapper.text()).toContain('7 天的扫描记录和 30 天的规划记录')
    expect(wrapper.text()).toContain('24 小时')
    expect(wrapper.text()).not.toContain('15 分钟')
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(8)
    expect(wrapper.findAll('input[type="checkbox"]').every((box) => (box.element as HTMLInputElement).checked)).toBe(true)
  })

  it('cannot start before a check and drops the summary on a selection change', async () => {
    const previewCleanup = vi.fn(async (categories: CleanupCategory[]) => preview({ scan_sessions: 3, works: 2 }, categories))
    const wrapper = await mountIt(apiStub({ previewCleanup }))
    expect(wrapper.get(tid('cleanup-start')).attributes('disabled')).toBeDefined()
    await wrapper.get(tid('cleanup-check')).trigger('click')
    await flushPromises()
    expect(previewCleanup).toHaveBeenCalledWith(ALL)
    expect(wrapper.get(tid('cleanup-candidates-works')).text()).toBe('2')
    expect(wrapper.get(tid('cleanup-start')).attributes('disabled')).toBeUndefined()

    await wrapper.get(tid('cleanup-select-works')).setValue(false)
    expect(wrapper.find(tid('cleanup-candidates-scan_sessions')).exists()).toBe(false)
    expect(wrapper.get(tid('cleanup-start')).attributes('disabled')).toBeDefined()
    await wrapper.get(tid('cleanup-check')).trigger('click')
    await flushPromises()
    expect(previewCleanup).toHaveBeenLastCalledWith(ALL.filter((id) => id !== 'works'))
  })

  it('reports nothing to clean and keeps start disabled', async () => {
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({})) }))
    await wrapper.get(tid('cleanup-check')).trigger('click')
    await flushPromises()
    expect(wrapper.get(tid('cleanup-none')).text()).toBe('无需清理')
    expect(wrapper.get(tid('cleanup-start')).attributes('disabled')).toBeDefined()
  })

  it('confirms with categories and counts only, then starts once and shows the live run', async () => {
    const startCleanup = vi.fn(async () => run())
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({ scan_sessions: 3 })), startCleanup }))
    await wrapper.get(tid('cleanup-check')).trigger('click')
    await flushPromises()
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    const list = document.body.querySelector(tid('cleanup-confirm-list'))!
    expect(list.textContent).toContain('扫描记录3')
    expect(document.body.textContent).toContain('无法撤销')
    expect(document.body.querySelector('input[type="text"]')).toBeNull()
    ;(document.body.querySelector(tid('cleanup-confirm')) as HTMLElement).click()
    await flushPromises()
    expect(startCleanup).toHaveBeenCalledTimes(1)
    expect(wrapper.get(tid('cleanup-status')).text()).toBe('清理正在运行')
    expect(wrapper.get(tid('cleanup-count-scan_sessions')).text()).toContain('541')
    expect(wrapper.find(tid('cleanup-check')).exists()).toBe(false)
  })

  it('shows a run found on return and stops it', async () => {
    const cancelCleanup = vi.fn(async () => run({ status: 'stopping' }))
    const wrapper = await mountIt(apiStub({ getCleanup: vi.fn().mockResolvedValue(run()), cancelCleanup }))
    expect(wrapper.get(tid('cleanup-status')).text()).toBe('清理正在运行')
    await wrapper.get(tid('cleanup-stop')).trigger('click')
    await flushPromises()
    expect(cancelCleanup).toHaveBeenCalledWith('r1')
    expect(wrapper.get(tid('cleanup-status')).text()).toContain('正在停止')
    expect(wrapper.get(tid('cleanup-stop')).attributes('disabled')).toBeDefined()
  })

  it.each([
    ['stopped', '已按请求停止'], ['succeeded', '已完成'], ['failed', '失败'], ['interrupted', '已中断'],
  ] as const)('keeps committed counts on a %s run and allows a new check', async (status, text) => {
    const wrapper = await mountIt(apiStub({ getCleanup: vi.fn().mockResolvedValue(run({ status, finished_at: '2026-10-23T05:00:00Z' })) }))
    expect(wrapper.get(tid('cleanup-status')).text()).toBe(text)
    expect(wrapper.get(tid('cleanup-count-scan_sessions')).text()).toContain('541')
    expect(wrapper.get(tid('cleanup-check')).attributes('disabled')).toBeUndefined()
  })

  it('on BUSY reads the global run instead of resending, without claiming it is ours', async () => {
    const startCleanup = vi.fn().mockRejectedValue(new ApiError(409, 'BUSY', 'busy'))
    const getCleanup = vi.fn().mockResolvedValueOnce(null).mockResolvedValue(run())
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({ works: 1 })), startCleanup, getCleanup }))
    await wrapper.get(tid('cleanup-check')).trigger('click')
    await flushPromises()
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    ;(document.body.querySelector(tid('cleanup-confirm')) as HTMLElement).click()
    await flushPromises()
    expect(startCleanup).toHaveBeenCalledTimes(1)
    expect(getCleanup).toHaveBeenCalledTimes(2)
    expect(wrapper.get(tid('cleanup-status')).text()).toBe('清理正在运行')
  })

  it('treats a lost start response as unknown and only queries status', async () => {
    const startCleanup = vi.fn().mockRejectedValue(new TypeError('network'))
    const getCleanup = vi.fn().mockResolvedValue(null)
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({ works: 1 })), startCleanup, getCleanup }))
    await wrapper.get(tid('cleanup-check')).trigger('click')
    await flushPromises()
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    ;(document.body.querySelector(tid('cleanup-confirm')) as HTMLElement).click()
    await flushPromises()
    expect(startCleanup).toHaveBeenCalledTimes(1)
    expect(getCleanup).toHaveBeenCalledTimes(2)
    expect(wrapper.get(tid('cleanup-error')).text()).toContain('无法确认')
  })

  it('marks the state unknown when the status read fails', async () => {
    const wrapper = await mountIt(apiStub({ getCleanup: vi.fn().mockRejectedValue(new TypeError('offline')) }))
    expect(wrapper.get(tid('cleanup-unknown')).text()).toContain('状态未知')
  })
})
