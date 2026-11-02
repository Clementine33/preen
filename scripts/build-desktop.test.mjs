import assert from 'node:assert/strict'
import { test } from 'node:test'
import path from 'node:path'
import { releaseVersion, webOutputDir } from './build-desktop.mjs'

test('desktop web copy lands where the shell and service read it', () => {
  assert.equal(webOutputDir('out', 'windows'), path.join('out', 'web'))
  assert.equal(webOutputDir('out', 'linux'), path.join('out', 'share', 'preen', 'web'))
})

test('desktop release versions reject missing, malformed and development versions', () => {
  for (const raw of ['', '1.2', 'v01.2.3', '1.2.3-01', '1.2.3-dev', '1.2.3-rc.dev.1+build', '1.2.3+dev', '1.2.3-beta.1', 'v1.2.3-rc.01']) {
    assert.throws(() => releaseVersion(raw), /RAW_TAG/)
  }
  assert.equal(releaseVersion('v1.2.3'), '1.2.3')
  assert.equal(releaseVersion('1.2.3-rc.1'), '1.2.3-rc.1')
  assert.equal(releaseVersion('v1.2.3-rc.0'), '1.2.3-rc.0')
})
