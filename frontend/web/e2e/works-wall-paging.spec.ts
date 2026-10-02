import { test, expect, type Page } from '@playwright/test'
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'

// The wall at a size the dev library never reaches: more than five keyset
// pages of works. An empty directory named for a work is already a collected
// copy, so the listing needs no catalog metadata (and no network).
const TOTAL = 260
const PAGE = 50

/** The header's WORKS count: what has loaded so far (the listing has no total). */
async function loadedCount(page: Page): Promise<number> {
  return Number(await page.locator('.count b').innerText())
}

test('the works wall pages through a large library and stays virtualized', async ({ page, request }) => {
  test.skip(process.env.BROWSER_E2E !== '1', 'requires the real backend stack')
  const state = JSON.parse(readFileSync(process.env.ONSEI_E2E_STATE_FILE ?? path.resolve('e2e/.stack-state.json'), 'utf8'))
  const api = `http://127.0.0.1:${state.httpPort}/api/v1`
  const root = mkdtempSync(path.join(os.tmpdir(), 'onsei-wall-'))
  for (let i = 0; i < TOTAL; i++) mkdirSync(path.join(root, `RJ0${String(1_100_000 + i)}`))
  let libraryId: string | undefined
  try {
    const created = await request.post(`${api}/libraries`, { data: { name: 'Wall paging', root_path: root } })
    expect(created.ok()).toBe(true)
    libraryId = (await created.json()).id
    const scan = await request.post(`${api}/libraries/${libraryId}/scans`, { data: {} })
    expect(await scan.text()).toContain('event: completed')

    // An ordinary screen: one page fills it, and each scroll to the bottom
    // brings the next one.
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/works?library=${libraryId}`)
    await expect.poll(() => loadedCount(page)).toBe(PAGE)
    const wall = page.getByTestId('work-wall')
    await expect.poll(async () => {
      await wall.evaluate((el) => el.scrollTo(0, el.scrollHeight))
      return loadedCount(page)
    }, { timeout: 30_000 }).toBe(TOTAL)
    // Every work is loaded, but only the rows around the viewport are cells.
    expect(await wall.locator('[data-rj]').count()).toBeLessThan(PAGE)

    // A screen the first page does not fill: nothing scrolls, and the wall
    // still asks for pages until it does.
    await page.setViewportSize({ width: 2560, height: 2400 })
    await page.reload()
    await expect.poll(() => loadedCount(page)).toBeGreaterThan(PAGE)
  } finally {
    await page.goto('/worksets').catch(() => {})
    if (libraryId) await request.delete(`${api}/libraries/${libraryId}`)
    rmSync(root, { recursive: true, force: true })
  }
})
