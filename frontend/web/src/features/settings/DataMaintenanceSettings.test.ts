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
const body = (id: string) => document.body.querySelector<HTMLElement>(tid(id))

describe('data maintenance', () => {
  it('states only the automatic retention periods and schedule', async () => {
    const wrapper = await mountIt(apiStub())
    expect(wrapper.text()).toContain('7 天的扫描记录和 30 天的规划记录')
    expect(wrapper.text()).toContain('24 小时')
    expect(wrapper.text()).not.toContain('15 分钟')
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(8)
    expect(wrapper.findAll('input[type="checkbox"]').every((box) => (box.element as HTMLInputElement).checked)).toBe(true)
  })

  it('previews the current selection when start is pressed', async () => {
    const previewCleanup = vi.fn(async (categories: CleanupCategory[]) => preview({ scan_sessions: 3, works: 2 }, categories))
    const wrapper = await mountIt(apiStub({ previewCleanup }))
    expect(previewCleanup).not.toHaveBeenCalled()
    await wrapper.get(tid('cleanup-select-works')).setValue(false)
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    expect(previewCleanup).toHaveBeenCalledWith(ALL.filter((id) => id !== 'works'))
    expect(document.body.querySelector(tid('cleanup-confirm-total'))!.textContent).toContain('将清理 3 条记录')
  })

  it('cannot start with nothing selected', async () => {
    const wrapper = await mountIt(apiStub())
    for (const id of ALL) await wrapper.get(tid(`cleanup-select-${id}`)).setValue(false)
    expect(wrapper.get(tid('cleanup-start')).attributes('disabled')).toBeDefined()
  })

  it('reports nothing to clean and keeps confirm disabled', async () => {
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({})) }))
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    expect(document.body.querySelector(tid('cleanup-none'))!.textContent).toBe('无需清理')
    expect((document.body.querySelector(tid('cleanup-confirm')) as HTMLButtonElement).disabled).toBe(true)
  })

  it('confirms with the total only, then starts once and shows the live run in the dialog', async () => {
    const startCleanup = vi.fn(async () => run())
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({ scan_sessions: 3 })), startCleanup }))
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    expect(document.body.querySelector(tid('cleanup-confirm-total'))!.textContent).toContain('将清理 3 条记录')
    expect(document.body.textContent).toContain('无法撤销')
    expect(document.body.querySelector('input[type="text"]')).toBeNull()
    ;(document.body.querySelector(tid('cleanup-confirm')) as HTMLElement).click()
    await flushPromises()
    expect(startCleanup).toHaveBeenCalledTimes(1)
    expect(body('cleanup-status')!.textContent).toBe('清理正在运行')
    expect(body('cleanup-count-scan_sessions')!.textContent).toContain('541')
    expect(wrapper.find(tid('cleanup-start')).exists()).toBe(false)
    expect(wrapper.find(tid('cleanup-run')).exists()).toBe(false)
  })

  it('shows its own run\'s result once it ends', async () => {
    const done = run({ status: 'succeeded', finished_at: '2026-10-23T05:00:00Z' })
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({ scan_sessions: 3 })), startCleanup: vi.fn(async () => done) }))
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    body('cleanup-confirm')!.click()
    await flushPromises()
    expect(body('cleanup-status')!.textContent).toBe('已完成')
    expect(body('cleanup-count-scan_sessions')!.textContent).toContain('541')
  })

  it('offers a run found on return and stops it from the dialog', async () => {
    const cancelCleanup = vi.fn(async () => run({ status: 'stopping' }))
    const wrapper = await mountIt(apiStub({ getCleanup: vi.fn().mockResolvedValue(run()), cancelCleanup }))
    expect(wrapper.get(tid('cleanup-active')).text()).toContain('清理正在运行')
    expect(wrapper.find(tid('cleanup-start')).exists()).toBe(false)
    await wrapper.get(tid('cleanup-show')).trigger('click')
    await flushPromises()
    body('cleanup-stop')!.click()
    await flushPromises()
    expect(cancelCleanup).toHaveBeenCalledWith('r1')
    expect(body('cleanup-status')!.textContent).toContain('正在停止')
    expect((body('cleanup-stop') as HTMLButtonElement).disabled).toBe(true)
  })

  it.each([
    ['stopped'], ['succeeded'], ['failed'], ['interrupted'],
  ] as const)('never shows an earlier %s run on the page', async (status) => {
    const wrapper = await mountIt(apiStub({ getCleanup: vi.fn().mockResolvedValue(run({ status, finished_at: '2026-10-23T05:00:00Z' })) }))
    expect(document.body.querySelector(tid('cleanup-status'))).toBeNull()
    expect(document.body.querySelector(tid('cleanup-run'))).toBeNull()
    expect(wrapper.get(tid('cleanup-start')).attributes('disabled')).toBeUndefined()
  })

  it('on BUSY reads the global run instead of resending, without claiming it is ours', async () => {
    const startCleanup = vi.fn().mockRejectedValue(new ApiError(409, 'BUSY', 'busy'))
    const getCleanup = vi.fn().mockResolvedValueOnce(null).mockResolvedValue(run())
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({ works: 1 })), startCleanup, getCleanup }))
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    ;(document.body.querySelector(tid('cleanup-confirm')) as HTMLElement).click()
    await flushPromises()
    expect(startCleanup).toHaveBeenCalledTimes(1)
    expect(getCleanup).toHaveBeenCalledTimes(2)
    expect(body('cleanup-status')!.textContent).toBe('清理正在运行')
  })

  it('treats a lost start response as unknown and only queries status', async () => {
    const startCleanup = vi.fn().mockRejectedValue(new TypeError('network'))
    const getCleanup = vi.fn().mockResolvedValue(null)
    const wrapper = await mountIt(apiStub({ previewCleanup: vi.fn(async () => preview({ works: 1 })), startCleanup, getCleanup }))
    await wrapper.get(tid('cleanup-start')).trigger('click')
    await flushPromises()
    ;(document.body.querySelector(tid('cleanup-confirm')) as HTMLElement).click()
    await flushPromises()
    expect(startCleanup).toHaveBeenCalledTimes(1)
    expect(getCleanup).toHaveBeenCalledTimes(2)
    expect(body('cleanup-error')!.textContent).toContain('无法确认')
  })

  it('marks the state unknown when the status read fails', async () => {
    const wrapper = await mountIt(apiStub({ getCleanup: vi.fn().mockRejectedValue(new TypeError('offline')) }))
    expect(wrapper.get(tid('cleanup-unknown')).text()).toContain('状态未知')
  })
})
