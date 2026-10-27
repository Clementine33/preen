import { mkdirSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { expect, test } from '@playwright/test'

// The wall at a size the dev library never reaches: five numbered
// pages of works. An empty directory named for a work is already a collected
// copy, so the listing needs no catalog metadata (and no network).
const TOTAL = 260
const PAGE = 60

test('the works wall pages through a large library and keeps the rendered range bounded', async ({ page, request }) => {
  test.skip(process.env.BROWSER_E2E !== '1', 'requires the real backend stack')
  const state = JSON.parse(readFileSync(process.env.PREEN_E2E_STATE_FILE ?? path.resolve('e2e/.stack-state.json'), 'utf8'))
  const api = `http://127.0.0.1:${state.httpPort}/api/v1`
  const root = mkdtempSync(path.join(os.tmpdir(), 'onsei-wall-'))
  for (let i = 0; i < TOTAL; i++) mkdirSync(path.join(root, `RJ0${String(1_100_000 + i)}`))
  let libraryId: string | undefined
  try {
    const created = await request.post(`${api}/libraries`, {
      data: { name: 'Wall paging', root_path: root },
    })
    expect(created.ok()).toBe(true)
    libraryId = (await created.json()).id
    const scan = await request.post(`${api}/libraries/${libraryId}/scans`, { data: {} })
    expect(await scan.text()).toContain('event: completed')

    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/works?library=${libraryId}&size=${PAGE}`)
    await expect(page.locator('.count b')).toHaveText(String(TOTAL))
    const wall = page.getByTestId('work-wall')
    const pager = page.getByRole('navigation', { name: 'Pages' })
    const pages = Math.ceil(TOTAL / PAGE)
    await expect(wall.locator('[data-rj]')).toHaveCount(PAGE)
    await expect(pager.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page')
    const first = await wall.locator('[data-rj]').first().getAttribute('data-rj')
    await pager.getByRole('button', { name: 'Next page' }).click()
    await expect(page).toHaveURL(/page=2(?:&|$)/)
    await expect(pager.getByRole('button', { name: 'Page 2', exact: true })).toHaveAttribute('aria-current', 'page')
    await expect(wall.locator('[data-rj]').first()).not.toHaveAttribute('data-rj', first!)
    await expect(wall.locator('[data-rj]')).toHaveCount(PAGE)

    // A direct link and a reload retain the last page, independent of viewport.
    await page.goto(`/works?library=${libraryId}&size=${PAGE}&page=${pages}`)
    await expect(wall.locator('[data-rj]')).toHaveCount(TOTAL % PAGE)
    await expect(pager.getByRole('button', { name: 'Next page' })).toBeDisabled()

    // A larger viewport and a reload keep the same last-page range.
    await page.setViewportSize({ width: 2560, height: 2400 })
    await page.reload()
    await expect(wall.locator('[data-rj]')).toHaveCount(TOTAL % PAGE)
    await expect(pager.getByRole('button', { name: `Page ${pages}`, exact: true })).toHaveAttribute('aria-current', 'page')
  }
  finally {
    await page.goto('/worksets').catch(() => {})
    if (libraryId)
      await request.delete(`${api}/libraries/${libraryId}`)
    rmSync(root, { recursive: true, force: true })
  }
})
