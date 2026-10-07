import { expect, test, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'

/**
 * UI execution smoke against real media: start a revision, read its live
 * result, reload the report and verify the spent revision cannot run again.
 * Recovery placement, hard deletion, scope, codecs and cancel semantics live
 * in the backend HTTP black-box tests.
 */

const e2eEnabled = process.env.BROWSER_E2E === '1'

test.describe.configure({ timeout: 240_000 })

const fixtures: string[] = []

function makeFixtureRoot(prefix: string): string {
  const root = mkdtempSync(path.join(os.tmpdir(), `onsei-e2e-${prefix}-`))
  fixtures.push(root)
  return root
}

test.afterEach(() => {
  for (const root of fixtures.splice(0)) rmSync(root, { recursive: true, force: true })
})

function run(tool: string, args: string[]): string {
  return execFileSync(tool, args, { encoding: 'utf8' })
}

/** A real tone album: 00.wav source, a below-target 128k 00.mp3, optional aac. */
function encodeAlbum(root: string, name: string, opts: { m4a?: boolean } = {}): string {
  const dir = path.join(root, name)
  mkdirSync(dir, { recursive: true })
  const wav = path.join(dir, '00.wav')
  run('ffmpeg', ['-nostdin', '-v', 'error', '-y', '-f', 'lavfi', '-i', 'sine=frequency=440:duration=6', '-c:a', 'pcm_s16le', wav])
  run('ffmpeg', ['-nostdin', '-v', 'error', '-y', '-i', wav, '-b:a', '128k', path.join(dir, '00.mp3')])
  if (opts.m4a) {
    run('ffmpeg', ['-nostdin', '-v', 'error', '-y', '-i', wav, '-c:a', 'aac', '-b:a', '128k', path.join(dir, '00.m4a')])
  }
  return dir
}

function mp3Bitrate(file: string): number {
  // ffprobe takes no -nostdin here (unlike ffmpeg); the Go e2e probes the same way.
  const raw = run('ffprobe', ['-v', 'error', '-select_streams', 'a:0', '-show_entries', 'stream=bit_rate', '-of', 'json', file])
  const parsed = JSON.parse(raw) as { streams: Array<{ bit_rate: string }> }
  return Number(parsed.streams[0]?.bit_rate ?? 0)
}

async function addLibraryAndScan(page: Page, root: string, name: string): Promise<void> {
  await page.goto('/worksets')
  const emptyState = page.getByTestId('empty-add-library')
  if (await emptyState.isVisible().catch(() => false)) {
    await emptyState.click()
  } else {
    await page.getByRole('button', { name: '添加媒体库' }).first().click()
  }
  await page.locator('#library-name').fill(name)
  await page.locator('#library-root').fill(root)
  await page.getByRole('button', { name: '保存' }).click()
  // Entering the library opens its workbench, where it is scanned.
  await page.getByRole('main').getByRole('link', { name: new RegExp(name) }).first().click()
  await expect(page).toHaveURL(/\/worksets\/[^/]+$/)
  await page.getByTestId('scan-button').click()
  await expect(page.getByText('扫描完成')).toBeVisible({ timeout: 60_000 })
}

/** Wait for the draft write itself; applying temporarily disables the button too. */
async function applyCommonSettings(page: Page): Promise<void> {
  const [response] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'PUT' && response.url().endsWith('/conversion/draft')),
    page.getByTestId('apply-common').click(),
  ])
  expect(response.ok()).toBe(true)
  await expect(page.getByTestId('apply-common')).toBeDisabled()
}

/** Creates the library's conversion record over every scanned folder. */
async function createWorksetForAllFolders(page: Page): Promise<void> {
  await page.getByRole('checkbox', { name: '选择全部文件夹' }).check()
  await page.getByTestId('enter-conversion').click()
  await expect(page).toHaveURL(/\/conversion$/)
  // Common settings are deployment-wide; each case starts from the defaults.
  await page.getByTestId('nav-conversion-settings').click()
  await expect(page.getByTestId('common-settings-form')).toBeVisible()
  for (const unit of ['mode', 'no_effect_tags', 'matched', 'unmatched']) {
    const restore = page.getByTestId(`common-${unit}-restore`)
    if (await restore.isEnabled()) await restore.click()
  }
  await page.getByTestId('common-delete-mode-soft').check()
  const apply = page.getByTestId('apply-common')
  if (await apply.isEnabled()) await applyCommonSettings(page)
  await expect(apply).toBeDisabled()
  await page.getByRole('link', { name: '← 转换列表' }).click()
}

async function generatePlan(page: Page): Promise<void> {
  await page.getByTestId('start-generation').click()
  await expect(page.getByTestId('operation-counts')).toBeVisible({ timeout: 60_000 })
}

/** Starts the run straight from the header — the mode is a settings choice. */
async function startExecution(page: Page): Promise<void> {
  await expect(page.getByTestId('start-execution')).toBeVisible({ timeout: 15_000 })
  await page.getByTestId('start-execution').click()
  // The run opens its own detail carrier instead of crowding the member list.
  await expect(page).toHaveURL(/\/conversion\/execution$/)
  await expect(page.getByTestId('execution-panel')).toBeVisible({ timeout: 15_000 })
}

test.describe('workset execution', () => {
  test.skip(!e2eEnabled, 'execution e2e runs only with BROWSER_E2E=1')

  test('execute the current plan and restore the finished report after reload', async ({ page }) => {
    const root = makeFixtureRoot('exec-soft')
    const album = encodeAlbum(root, 'RJ000001 soft-album', { m4a: true })

    await addLibraryAndScan(page, root, 'E2E Exec Soft')
    await createWorksetForAllFolders(page)
    await generatePlan(page)
    await startExecution(page)

    await expect(page.getByTestId('execution-status')).toHaveText('已完成', { timeout: 90_000 })
    // The report lives in the detail carrier, not in the middle of the list.
    await expect(page.getByTestId('workbench-detail-inline').getByTestId('execution-panel')).toBeVisible()
    await expect(page.getByTestId('execution-panel')).toContainText('已写入')
    await expect(page.getByTestId('execution-panel')).toContainText('已清理到 Delete/')
    // 保留文件 is the plan's conclusion, not the run's bookkeeping: the declared
    // lossless target was already satisfied, so it stayed untouched — and the
    // Delete/ destinations the run moved files to are not listed at all.
    await expect(page.getByTestId('execution-kept')).toContainText('00.wav')
    await expect(page.getByTestId('execution-panel')).not.toContainText('Delete/RJ000001 soft-album')

    // One disk smoke: the UI-triggered session really rebuilt its output.
    expect(mp3Bitrate(path.join(album, '00.mp3'))).toBeGreaterThanOrEqual(319_000)

    // A reload restores the terminal session from the server (no live stream
    // survives it) on the same report page, and the revision can never run
    // again.
    await page.reload()
    await expect(page).toHaveURL(/\/conversion\/execution$/)
    await expect(page.getByTestId('execution-status')).toHaveText('已完成', { timeout: 30_000 })
    const executeAgain = page.getByTestId('start-execution')
    await expect(executeAgain).toBeVisible()
    await expect(executeAgain).toBeDisabled()
    await expect(executeAgain).toHaveAttribute('title', /已执行/)
  })

})
