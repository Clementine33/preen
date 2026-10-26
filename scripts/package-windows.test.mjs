import assert from 'node:assert/strict'
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { test } from 'node:test'
import { packageWindows, requiredLicenses } from './package-windows.mjs'

test('portable assembly validates every input and archives only the fixed layout', (t) => {
  const root = mkdtempSync(path.join(os.tmpdir(), 'preen-portable-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  cpSync(new URL('../mise.toml', import.meta.url), path.join(root, 'mise.toml'))
  const inputs = ['LICENSE', 'dist/desktop/windows-amd64/preen-desktop.exe', 'dist/desktop/windows-amd64/bin/preen.exe',
    ...['ffmpeg.exe', 'ffprobe.exe'].map((name) => `dist/desktop-ffmpeg/bin/${name}`),
    ...requiredLicenses(root).map((name) => `dist/desktop-ffmpeg/licenses/${name}`)]
  for (const input of inputs) {
    mkdirSync(path.dirname(path.join(root, input)), { recursive: true })
    writeFileSync(path.join(root, input), 'fixture')
  }
  const options = { root, rawTag: 'v1.2.3', commit: 'a'.repeat(40), archive: () => assert.fail('must fail before archiving') }
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
    assert.deepEqual(readdirSync(stage).sort(), ['bin', 'licenses', 'preen-desktop.exe'])
    assert.deepEqual(readdirSync(path.join(stage, 'bin')).sort(), ['ffmpeg.exe', 'ffprobe.exe', 'preen.exe'])
    assert.equal(existsSync(path.join(stage, 'data')), false)
    const readme = readFileSync(path.join(stage, 'licenses/Preen-README.txt'), 'utf8')
    assert.match(readme, /Preen 1\.2\.3/)
    assert.match(readme, new RegExp(options.commit))
    for (const name of requiredLicenses(root)) assert.equal(existsSync(path.join(stage, 'licenses', name)), true)
    assert.equal(existsSync(path.join(stage, 'licenses/Preen-GPL-3.0.txt')), true)
    writeFileSync(destination, 'archive fixture')
  } })
  assert.equal(output, path.join(root, 'dist/preen-1.2.3-windows-amd64.zip'))
  assert.equal(readFileSync(output, 'utf8'), 'archive fixture')
  assert.equal(existsSync(`${output}.tmp`), false)
})
