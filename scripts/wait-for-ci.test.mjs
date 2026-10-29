import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'

const root = fileURLToPath(new URL('../', import.meta.url))
const harness = `
gh() {
  printf '%s\\n' "$*" >> "$CI_TEST_LOG"
  case "$2" in
    list)
      if [ "$CI_TEST_MODE" = api-error ]; then return 1; fi
      if [ "$CI_TEST_MODE" = missing ]; then return 0; fi
      if [ "$CI_TEST_MODE" = delayed ] && [ "$(wc -l < "$CI_TEST_LOG")" -eq 1 ]; then return 0; fi
      echo 42 ;;
    watch) return "$CI_TEST_WATCH_EXIT" ;;
    view) echo "$CI_TEST_CONCLUSION" ;;
    *) return 1 ;;
  esac
}
sleep() { :; }
export -f gh sleep
bash scripts/wait-for-ci.sh tagged-commit
`

for (const [mode, conclusion, watchExit, expectedSuccess] of [
  ['found', 'success', 0, true],
  ['delayed', 'success', 0, true],
  ['found', 'failure', 1, false],
  ['found', 'cancelled', 0, false],
  ['found', 'skipped', 0, false],
  ['missing', '', 0, false],
  ['api-error', '', 0, false],
]) {
  test(`release CI gate: ${mode}, ${conclusion || 'no conclusion'}`, (t) => {
    const dir = mkdtempSync(path.join(tmpdir(), 'preen-ci-gate-'))
    t.after(() => rmSync(dir, { recursive: true, force: true }))
    const log = path.join(dir, 'calls')
    const result = spawnSync('bash', ['-c', harness], {
      cwd: root,
      env: { ...process.env, CI_TEST_LOG: log, CI_TEST_MODE: mode, CI_TEST_CONCLUSION: conclusion, CI_TEST_WATCH_EXIT: String(watchExit) },
      encoding: 'utf8',
    })
    assert.ifError(result.error)
    assert.equal(result.status === 0, expectedSuccess, result.stderr)
    const calls = readFileSync(log, 'utf8').trim().split('\n')
    const lookups = calls.filter((call) => call.startsWith('run list'))
    assert.equal(lookups.length, mode === 'missing' ? 12 : mode === 'delayed' ? 2 : 1)
    for (const lookup of lookups) {
      assert.equal(lookup, 'run list --workflow ci.yml --commit tagged-commit --branch main --event push --limit 1 --json databaseId --jq .[0].databaseId // empty')
    }
    if (mode === 'missing' || mode === 'api-error') {
      assert.equal(calls.length, lookups.length)
    } else {
      assert(calls.includes('run watch 42 --exit-status --interval 15'))
    }
  })
}
