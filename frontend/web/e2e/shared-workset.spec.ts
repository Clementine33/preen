import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { expect, test } from '@playwright/test'
import { readStackState } from './helpers/stack-state.ts'

test('select one batch and reuse its members and independent plans across tasks', async ({ page }) => {
  test.skip(process.env.BROWSER_E2E !== '1', 'requires the real stack')
  const root = mkdtempSync(path.join(os.tmpdir(), 'onsei-shared-batch-'))
  const folders = ['RJ000001 audio', 'RJ000002 empty']
  try {
    for (const folder of folders) mkdirSync(path.join(root, folder))
    writeFileSync(path.join(root, folders[0], 'track.mp3'), 'dummy audio')
    await page.goto('/worksets')
    await page.getByRole('button', { name: '添加媒体库', exact: true }).first().click()
    await page.locator('#library-name').fill('Shared batch')
    await page.locator('#library-root').fill(root)
    await page.getByRole('button', { name: '保存', exact: true }).click()
    await page.getByRole('main').getByRole('link', { name: /Shared batch/ }).first().click()
    await expect(page).toHaveURL(/\/worksets\/[^/]+$/)
    await expect(page.getByTestId('overview')).toBeVisible()
    const overview = page.url()
    const libraryId = new URL(overview).pathname.split('/').at(-1)!
    await page.getByTestId('scan-button').click()
    await expect(page.getByText('扫描完成')).toBeVisible()
    for (const folder of folders) await page.getByRole('checkbox', { name: `选择 ${folder}`, exact: true }).first().check()

    let creations = 0
    page.on('request', (request) => {
      if (request.method() === 'PUT' && request.url().endsWith('/current'))
        creations++
    })
    await page.getByTestId('enter-conversion').click()
    await expect(page).toHaveURL(/\/conversion$/)
    const { httpPort } = readStackState()
    const api = `http://127.0.0.1:${httpPort}/api/v1`
    const readRecord = async (kind: string) => {
      const response = await page.request.get(`${api}/libraries/${libraryId}/operations/${kind}/current`)
      expect(response.status(), await response.text()).toBe(200)
      return (await response.json()).workset
    }
    const batch = await readRecord('conversion')
    // The workset view holds no operations: a task's plan is read from its own operation.
    const readPlanId = async (kind: string) => {
      const response = await page.request.get(`${api}/worksets/${batch.workset_id}/operations/${kind}`)
      expect(response.status(), await response.text()).toBe(200)
      return (await response.json()).current_revision.plan_id as string
    }
    for (const kind of ['rename', 'metadata_refresh']) {
      const record = await readRecord(kind)
      expect(record.workset_id).toBe(batch.workset_id)
      expect(record.members).toEqual(batch.members)
    }

    await page.goto(overview)
    await page.getByTestId('nav-rename').click()
    await expect(page.getByTestId('task-page-rename')).toContainText('2 个文件夹')
    await page.goto(overview)
    await page.getByTestId('nav-metadata-refresh').click()
    await expect(page.getByTestId('task-page-metadata_refresh')).toContainText('2 个文件夹')
    await page.getByTestId('generate').click()
    await expect(page.getByTestId('metadata-plan-count')).toContainText('将刷新 2 个')
    const metadataPlanId = await readPlanId('metadata_refresh')

    await page.goto(overview)
    await page.getByTestId('nav-conversion').click()
    await expect(page.getByTestId('member-toolbar')).toBeVisible()
    await page.getByTestId('start-generation').click()
    await expect(page.getByTestId('operation-counts')).toBeVisible()
    const after = await readRecord('conversion')
    expect(after.members).toEqual(batch.members)
    expect(await readPlanId('metadata_refresh')).toBe(metadataPlanId)
    expect(creations).toBe(1)
  }
  finally {
    rmSync(root, { recursive: true, force: true })
  }
})
