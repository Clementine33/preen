#!/usr/bin/env node
/**
 * e2e/launch-stack.mjs — boot the real stack for the Playwright e2e smoke:
 * Go backend (fresh temp PREEN_DATA_DIR + generated fixture tree) + Vite dev
 * server wired to the backend's handshake http_port.
 *
 * Reuses the exact backend-launch discipline from scripts/dev-web.mjs:
 *   - the backend's stdin is a pipe WE hold (never inherit), because the
 *     backend watches stdin and shuts down on EOF, which would kill it
 *     instantly in any non-interactive environment (CI, pipes, background);
 *   - the additive PREEN_BACKEND_READY handshake is parsed for http_port and
 *     Vite is launched with PREEN_BACKEND_URL=http://127.0.0.1:<http_port> for its /api proxy;
 *   - teardown closes the held pipe first (graceful EOF shutdown), then
 *     falls back to killing the process group.
 *
 * Writes a tiny state file (default e2e/.stack-state.json, override with
 * PREEN_E2E_STATE_FILE) that the Playwright spec reads to learn the fixture
 * root and http port.
 */
import { spawn } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = path.resolve(webDir, '..', '..')
const backendDir = path.join(repoRoot, 'backend', 'go')

const isWin = process.platform === 'win32'
const stateFile = process.env.PREEN_E2E_STATE_FILE
  ? path.resolve(process.env.PREEN_E2E_STATE_FILE)
  : path.join(path.dirname(fileURLToPath(import.meta.url)), '.stack-state.json')

// --- Fixture tree -----------------------------------------------------------
// RJ-named Work Roots: albumA holds matching lossy/lossless stems,
// albumB holds only lossy audio. Both appear in the scanned folder listing.
function createFixtureTree() {
  const dataDir = mkdtempSync(path.join(os.tmpdir(), 'onsei-e2e-'))
  const rootPath = path.join(dataDir, 'music')
  const files = [
    ['RJ000001 albumA', 'test1.mp3'],
    ['RJ000001 albumA', 'test1.flac'],
    ['RJ000001 albumA', 'test2.mp3'],
    ['RJ000001 albumA', 'test2.flac'],
    ['RJ000002 albumB', 'song1.mp3'],
    ['RJ000002 albumB', 'song2.mp3'],
  ]
  for (const [folder, file] of files) {
    const dir = path.join(rootPath, folder)
    mkdirSync(dir, { recursive: true })
    writeFileSync(path.join(dir, file), 'dummy audio')
  }
  return rootPath
}

const fixtureRoot = createFixtureTree()

// --- Backend ----------------------------------------------------------------
const dataDir = mkdtempSync(path.join(os.tmpdir(), 'onsei-e2e-data-'))
// Seed the classifier tag config: no compiled-in defaults remain, so fresh
// data dirs get their literal tags from config.json.
writeFileSync(
  path.join(dataDir, 'config.json'),
  JSON.stringify({
    prune: {
      no_effect_tags: ['SEなし'],
    },
  }),
)
const backend = spawn('go', ['run', './cmd/preen', '--watch-stdin'], {
  cwd: backendDir,
  env: { ...process.env, PREEN_DATA_DIR: dataDir },
  // Held stdin pipe: never inherit (see header comment).
  stdio: ['pipe', 'pipe', 'inherit'],
  // Own process group on POSIX so killing -pid takes the compiled binary
  // (a grandchild of `go run`) down with it.
  detached: !isWin,
})

let lineBuffer = ''
let vite = null
let teardownStarted = false

function launchVite(httpPort) {
  // Launch vite directly via its bin entry (node node_modules/vite/bin/vite.js)
  // instead of a pnpm shim: Windows .cmd/.exe shims behind cmd.exe have
  // unreliable argument pass-through (the --host 127.0.0.1 flag was dropped
  // in practice, leaving vite bound to ::1 only, which the IPv4 readiness
  // probe can never reach). A direct node spawn needs no shell and no shim.
  const viteBin = path.join(webDir, 'node_modules', 'vite', 'bin', 'vite.js')
  vite = spawn(process.execPath, [
    viteBin,
    '--port',
    process.env.PREEN_E2E_WEB_PORT,
    '--strictPort',
    '--host',
    '127.0.0.1',
  ], {
    cwd: webDir,
    env: { ...process.env, PREEN_BACKEND_URL: `http://127.0.0.1:${httpPort}` },
    stdio: 'inherit',
  })
  vite.on('exit', (code) => {
    console.log(`[e2e] Vite exited (code ${code}) — tearing down`)
    teardown(code ?? 0)
  })
  vite.on('error', (err) => {
    console.error(`[e2e] failed to launch Vite: ${err.message}`)
    teardown(1)
  })
}

function handleLine(line) {
  // PREEN_BACKEND_READY version=v1 http_port=54321
  const match = /^PREEN_BACKEND_READY version=\S+ http_port=(\d+)$/.exec(line.trim())
  if (!match)
    return
  const httpPort = Number(match[1])
  writeFileSync(stateFile, JSON.stringify({ fixtureRoot, httpPort }, null, 2))
  console.log(`[e2e] backend ready (http_port=${httpPort})`)
  console.log(`[e2e] fixture root: ${fixtureRoot}`)
  console.log(`[e2e] state file: ${stateFile}`)
  launchVite(httpPort)
}

backend.stdout.on('data', (chunk) => {
  lineBuffer += chunk.toString()
  let nl
  // eslint-disable-next-line no-cond-assign
  while ((nl = lineBuffer.indexOf('\n')) >= 0) {
    handleLine(lineBuffer.slice(0, nl))
    lineBuffer = lineBuffer.slice(nl + 1)
  }
})

backend.on('exit', (code) => {
  if (teardownStarted) {
    // Teardown is already in progress: finish() (rmSync cleanup then
    // process.exit) is scheduled and owns the exit. Force-exiting here
    // would skip the temp-dir cleanup.
    return
  }
  console.error(`[e2e] backend exited unexpectedly (code ${code})`)
  // Route through teardown so fixture/data/state cleanup and Vite shutdown
  // still run, preserving the unexpected exit code.
  teardown(code ?? 1)
})

backend.on('error', (err) => {
  console.error(`[e2e] failed to start backend: ${err.message}`)
  process.exit(1)
})

function teardown(exitCode = 0) {
  if (teardownStarted)
    return
  teardownStarted = true
  console.error(`[e2e] teardown(${exitCode}) — stopping stack`)

  const finish = () => {
    try {
      rmSync(path.dirname(fixtureRoot), { recursive: true, force: true })
      rmSync(dataDir, { recursive: true, force: true })
      rmSync(stateFile, { force: true })
      console.error('[e2e] teardown complete')
    }
    catch {
      /* best-effort cleanup */
    }
    process.exit(exitCode)
  }

  // Vite first: it is now a plain node child (no shim), but on Windows
  // killing the launcher pid would still orphan it, so taskkill /T takes the
  // whole tree; cleanup proceeds only after the tree is gone so tmp
  // dirs/state are not removed while children still hold them.
  const stopBackend = () => {
    if (backend.exitCode !== null)
      return finish()

    // Graceful stop: EOF on the pipe we hold makes the backend shut down.
    try {
      backend.stdin?.end()
    }
    catch {
      /* pipe may already be closed */
    }

    // EOF is the graceful stop on every platform, including Windows.
    const timeout = setTimeout(() => {
      if (isWin) {
        const kill = spawn('taskkill', [
          '/pid',
          String(backend.pid),
          '/t',
          '/f',
        ])
        kill.on('error', finish)
        kill.on('exit', finish)
      }
      else {
        try {
          process.kill(-backend.pid, 'SIGKILL')
        }
        catch {
          backend.kill('SIGKILL')
        }
      }
    }, 6000)
    backend.once('exit', () => { clearTimeout(timeout); finish() })
  }

  if (vite && vite.exitCode === null) {
    if (isWin) {
      const kill = spawn('taskkill', [
        '/pid',
        String(vite.pid),
        '/t',
        '/f',
      ])
      kill.on('error', stopBackend)
      kill.on('exit', stopBackend)
    }
    else {
      // Wait for Vite to actually exit before removing the fixture/data dirs,
      // so file watchers and in-flight writes are gone first. The timer is a
      // fallback for a Vite that does not terminate on SIGTERM.
      const viteGone = new Promise((resolve) => {
        vite.once('exit', resolve)
        vite.once('error', resolve)
        setTimeout(resolve, 1000)
      })
      vite.kill()
      void viteGone.then(stopBackend)
    }
  }
  else {
    stopBackend()
  }
}

process.on('SIGINT', () => teardown(130))
process.on('SIGTERM', () => teardown(143))

console.log(`[e2e] launching backend (data dir: ${dataDir})`)
