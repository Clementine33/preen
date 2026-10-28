import assert from 'node:assert/strict'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { test } from 'node:test'
import { packageWindows } from './package-windows.mjs'

test('portable assembly validates every input and archives only the fixed layout', (t) => {
  const root = mkdtempSync(path.join(os.tmpdir(), 'preen-portable-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const inputs = ['LICENSE', 'dist/desktop/windows-amd64/preen-desktop.exe', 'dist/desktop/windows-amd64/bin/preen.exe',
    ...['ffmpeg.exe', 'ffprobe.exe'].map((name) => `dist/desktop-ffmpeg/bin/${name}`),
    'dist/desktop-ffmpeg/THIRD_PARTY_NOTICES.txt']
  for (const input of inputs) {
    mkdirSync(path.dirname(path.join(root, input)), { recursive: true })
    writeFileSync(path.join(root, input), 'fixture')
  }
  const options = { root, rawTag: 'v1.2.3', archive: () => assert.fail('must fail before archiving') }
  for (const rawTag of ['', 'broken', '1.2.3-dev', '1.2.3-rc.dev.1']) {
    assert.throws(() => packageWindows({ ...options, rawTag }), /RAW_TAG/)
  }
  for (const input of inputs) {
    writeFileSync(path.join(root, input), '')
    assert.throws(() => packageWindows(options), /Missing or empty portable input/)
    rmSync(path.join(root, input))
    assert.throws(() => packageWindows(options), /Missing or empty portable input/)
    writeFileSync(path.join(root, input), 'fixture')
  }
  const stale = path.join(root, 'dist/portable-windows-amd64/data')
  mkdirSync(stale, { recursive: true })
  writeFileSync(path.join(stale, 'private.db'), 'never distribute')
  const output = packageWindows({ ...options, archive: (stage, destination) => {
    assert.deepEqual(readdirSync(stage).sort(), ['LICENSE', 'THIRD_PARTY_NOTICES.txt', 'bin', 'preen-desktop.exe'])
    assert.deepEqual(readdirSync(path.join(stage, 'bin')).sort(), ['ffmpeg.exe', 'ffprobe.exe', 'preen.exe'])
    assert.equal(existsSync(path.join(stage, 'data')), false)
    assert.equal(existsSync(path.join(stage, 'LICENSE')), true)
    assert.equal(existsSync(path.join(stage, 'THIRD_PARTY_NOTICES.txt')), true)
    writeFileSync(destination, 'archive fixture')
  } })
  assert.equal(output, path.join(root, 'dist/preen-1.2.3-windows-amd64.zip'))
  assert.equal(readFileSync(output, 'utf8'), 'archive fixture')
  assert.equal(existsSync(`${output}.tmp`), false)
})
