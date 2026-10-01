import { test, expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'

// The existing launcher supplies a fresh real backend. Only media fixtures and
// observation of the browser's native audio are added; no API/audio mocks.
test('work content updates from background measurement without restarting audio', async ({ page, request }) => {
  test.skip(process.env.BROWSER_E2E !== '1', 'requires the real backend stack')
  const state = JSON.parse(readFileSync(process.env.ONSEI_E2E_STATE_FILE ?? path.resolve('e2e/.stack-state.json'), 'utf8'))
  const api = `http://127.0.0.1:${state.httpPort}/api/v1`
  const root = mkdtempSync(path.join(os.tmpdir(), 'onsei-detail-'))
  const rj = 'RJ01000999'
  const member = path.join(root, rj)
  mkdirSync(member)
  const encode = (input: string, file: string) => execFileSync('ffmpeg', [
    '-nostdin', '-v', 'error', '-y', '-f', 'lavfi', '-i', input,
    ...(file.endsWith('.png') ? ['-frames:v', '1', '-threads', '1'] : []), path.join(member, file),
  ])
  let libraryId: string | undefined
  try {
    encode('color=c=blue:s=300x150', 'cover.png')
    encode('color=c=red:s=400x800', 'cover2.png')
    encode('sine=frequency=440:duration=90', '01_intro.wav')
    writeFileSync(path.join(member, '01_intro.lrc'), '[00:00.00]こんにちは\n[00:20.00]後半\n')
    // 「こんにちは」 in Shift_JIS, checked through the real byte route.
    writeFileSync(path.join(member, 'readme.txt'), Buffer.from([0x82, 0xb1, 0x82, 0xf1, 0x82, 0xc9, 0x82, 0xbf, 0x82, 0xcd]))
    const created = await request.post(`${api}/libraries`, { data: { name: 'Detail acceptance', root_path: root } })
    expect(created.ok()).toBe(true)
    libraryId = (await created.json()).id
    const scan = async () => {
      const result = await request.post(`${api}/libraries/${libraryId}/scans`, { data: {} })
      expect(result.ok()).toBe(true)
      expect(await result.text()).toContain('event: completed')
    }
    await scan()
    const content = await request.get(`${api}/works/${rj}/content`)
    expect(content.ok()).toBe(true)
    const manifest = await content.json()
    expect(manifest.copies).toHaveLength(1)
    const copy = manifest.copies[0].copy_id
    const range = await request.get(`${api}/works/${rj}/content/${copy}/01_intro.wav`, { headers: { Range: 'bytes=0-3' } })
    expect(range.status()).toBe(206)
    expect((await range.body()).toString()).toBe('RIFF')
    const text = await request.get(`${api}/works/${rj}/content/${copy}/readme.txt`)
    expect(text.headers()['content-type']).toContain('shift_jis')
    await page.addInitScript(() => {
      const nativePlay = HTMLMediaElement.prototype.play
      HTMLMediaElement.prototype.play = function () {
        Object.assign(window, { e2eAudio: this, e2ePlayCalls: ((window as unknown as { e2ePlayCalls?: number }).e2ePlayCalls ?? 0) + 1 })
        return nativePlay.call(this)
      }
    })
    await page.goto(`/works/${rj}`)
    await expect(page.locator('.thumb')).toHaveCount(2)
    await expect(page.locator('.thumb.pending')).toHaveCount(0)
    await page.locator('.thumb[title="cover.png"]').click()
    const original = await page.locator('.face.front').getAttribute('style')
    const thumbnail = await page.locator('.thumb[title="cover.png"]').getAttribute('style')
    expect(original).toContain('/content/')
    expect(original).toContain('?v=')
    expect(thumbnail).toContain('blob:')
    await page.locator('.bay-tabs .tab').filter({ hasText: 'FILES' }).click()
    await expect(page.locator('.row').filter({ hasText: 'cover.png' })).toContainText('300×150')
    await page.locator('.row').filter({ hasText: 'readme.txt' }).click()
    await expect(page.locator('.leaf')).toContainText('こんにちは')
    await page.locator('.bay-tabs .tab').filter({ hasText: 'TRACKS' }).click()
    await expect(page.locator('.trk').filter({ hasText: '01_intro' })).toContainText('01:30')
    await page.locator('.trk').filter({ hasText: '01_intro' }).click()
    // READ remains selected during playback; select LISTEN explicitly.
    await page.getByRole('button', { name: 'LISTEN', exact: true }).click()
    await page.locator('.lrc-line').filter({ hasText: '後半' }).click()
    await expect.poll(() => page.evaluate(() => (window as unknown as { e2eAudio: HTMLMediaElement }).e2eAudio.currentTime)).toBeGreaterThanOrEqual(20)
    await expect(page.locator('.baseline .bl-right')).toContainText('WAV')
    await page.evaluate(() => Object.assign(window, { e2eOriginalAudio: (window as unknown as { e2eAudio: HTMLMediaElement }).e2eAudio }))

    // Scan through another client: Vue mutation callbacks cannot refresh this
    // mounted page. The backend media-facts notification must do it.
    encode('color=c=green:s=600x300', 'cover.png')
    encode('color=c=white:s=200x100', 'cover3.png')
    encode('sine=duration=2', '02_extra.wav')
    await scan()
    await expect(page.locator('.thumb')).toHaveCount(3)
    await expect(page.locator('.thumb.pending')).toHaveCount(0)
    await expect(page.locator('.trk').filter({ hasText: '02_extra' })).toContainText('00:02')
    await expect.poll(() => page.locator('.face.front').getAttribute('style')).not.toBe(original)
    await expect.poll(() => page.locator('.thumb[title="cover.png"]').getAttribute('style')).not.toBe(thumbnail)
    expect(await page.evaluate(() => {
      const state = window as unknown as { e2eAudio: HTMLMediaElement; e2eOriginalAudio: HTMLMediaElement; e2ePlayCalls: number }
      return { same: state.e2eAudio === state.e2eOriginalAudio, time: state.e2eAudio.currentTime, calls: state.e2ePlayCalls }
    })).toMatchObject({ same: true, calls: 1 })
    await expect(page.locator('.baseline .t.pos')).not.toHaveText('00:00')
    await page.locator('.bay-tabs .tab').filter({ hasText: 'FILES' }).click()
    await expect(page.locator('.row').filter({ hasText: 'cover.png' })).toContainText('600×300')
  } finally {
    await page.evaluate(() => {
      const audio = (window as unknown as { e2eAudio?: HTMLMediaElement }).e2eAudio
      if (audio) { audio.pause(); audio.removeAttribute('src'); audio.load() }
    }).catch(() => {})
    await page.goto('/works')
    if (libraryId) await request.delete(`${api}/libraries/${libraryId}`)
    rmSync(root, { recursive: true, force: true })
  }
})
