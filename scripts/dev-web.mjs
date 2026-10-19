#!/usr/bin/env node
/**
 * dev-web.mjs — run the Go backend and the Vue/Vite web app together.
 *
 * Workflow:
 *   1. Spawn `go run ./cmd/preen` (backend/go module)
 *      with PREEN_DATA_DIR=<repo>/.dev_data so dev data never touches the
 *      real library location.
 *   2. Read the backend's stdout handshake:
 *        PREEN_BACKEND_READY version=<v> http_port=<http>
 *      and launch Vite with PREEN_BACKEND_URL=http://127.0.0.1:<http> for its /api proxy.
 *   3. On exit (SIGINT/SIGTERM, Vite exit, backend exit) tear down the
 *      backend first, cross-platform, using plain Node child processes —
 *      no shell scripting.
 *
 * The backend watches our held stdin pipe and shuts down when it closes.
 */
import { spawn } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const backendDir = path.join(repoRoot, 'backend', 'go')
const webDir = path.join(repoRoot, 'frontend', 'web')
const dataDir = path.join(repoRoot, '.dev_data')

const isWin = process.platform === 'win32'

mkdirSync(dataDir, { recursive: true })

const backend = spawn('go', ['run', './cmd/preen', '--watch-stdin'], {
  cwd: backendDir,
  env: { ...process.env, PREEN_DATA_DIR: dataDir },
  // We hold the backend's stdin open ourselves: the backend watches stdin
  // and shuts down on EOF (watchStdinEOF), so inheriting *our* stdin would
  // kill it instantly in any non-interactive environment (CI, IDE
  // integrated terminals, pipes, background jobs) where stdin is already at
  // EOF. An alive pipe of our own keeps it running everywhere and gives
  // teardown a graceful stop signal (close the pipe first, then fall back
  // to killing the process group).
  stdio: ['pipe', 'pipe', 'inherit'],
  // Own process group on POSIX so killing -pid takes the compiled binary
  // (a grandchild of `go run`) down with it.
  detached: !isWin,
})

let lineBuffer = ''
let vite = null
let teardownStarted = false

function launchVite(backendURL) {
  // Launch vite directly via its bin entry (node node_modules/vite/bin/vite.js)
  // instead of a pnpm shim: Windows pnpm installs may expose only pnpm.exe
  // (scoop shim, no pnpm.cmd), and even with a .cmd shim cmd.exe has
  // unreliable argument pass-through (the e2e launcher observed dropped
  // flags). A direct node spawn needs no shell and no shim.
  const viteBin = path.join(webDir, 'node_modules', 'vite', 'bin', 'vite.js')
  vite = spawn(process.execPath, [viteBin, '--host', '127.0.0.1'], {
    cwd: webDir,
    env: { ...process.env, PREEN_BACKEND_URL: backendURL },
    stdio: 'inherit',
  })
  vite.on('exit', (code) => {
    console.log(`[dev-web] Vite exited (code ${code}) — stopping backend`)
    teardown(code ?? 0)
  })
  vite.on('error', (err) => {
    console.error(`[dev-web] failed to launch Vite: ${err.message}`)
    teardown(1)
  })
}

function handleLine(line) {
  // PREEN_BACKEND_READY version=v1 http_port=54321
  const match = /^PREEN_BACKEND_READY version=\S+ http_port=(\d+)$/.exec(
    line.trim(),
  )
  if (!match) return
  const backendURL = `http://127.0.0.1:${match[1]}`
  console.log(`[dev-web] backend ready (http_port=${match[1]})`)
  console.log(`[dev-web] PREEN_BACKEND_URL=${backendURL}`)
  launchVite(backendURL)
}

backend.stdout.on('data', (chunk) => {
  lineBuffer += chunk.toString()
  let nl
  while ((nl = lineBuffer.indexOf('\n')) >= 0) {
    handleLine(lineBuffer.slice(0, nl))
    lineBuffer = lineBuffer.slice(nl + 1)
  }
})

backend.on('exit', (code) => {
  if (teardownStarted) {
    // Teardown owns the chosen exit code and waits for backend shutdown.
    return
  } else {
    console.error(`[dev-web] backend exited unexpectedly (code ${code})`)
    // Route through teardown so Windows process-tree cleanup (taskkill /T)
    // still runs and Vite is not left holding its port.
    teardown(code ?? 1)
  }
})

backend.on('error', (err) => {
  console.error(`[dev-web] failed to start backend: ${err.message}`)
  process.exit(1)
})

function teardown(exitCode = 0) {
  if (teardownStarted) return
  teardownStarted = true

  // Stop Vite first (it is the foreground piece; the backend is the
  // long-lived daemon that must not outlive us). Vite is now a plain node
  // child, so a direct kill works on every platform.
  if (vite && vite.exitCode === null) {
    vite.kill()
  }

  const finish = () => process.exit(exitCode)
  if (backend.exitCode !== null) return finish()

  // Graceful stop: closing the pipe we hold sends the backend an EOF on
  // its stdin watcher, which cancels and shuts it down on its own.
  try {
    backend.stdin?.end()
  } catch {
    /* pipe may already be closed */
  }

  // EOF is the graceful stop on every platform, including Windows.
  const timeout = setTimeout(() => {
    if (isWin) {
      const kill = spawn('taskkill', ['/pid', String(backend.pid), '/t', '/f'])
      kill.on('error', finish)
      kill.on('exit', finish)
    } else {
      try {
        process.kill(-backend.pid, 'SIGKILL')
      } catch {
        backend.kill('SIGKILL')
      }
    }
  }, 6000)
  backend.once('exit', () => { clearTimeout(timeout); finish() })
}

process.on('SIGINT', () => teardown(130))
process.on('SIGTERM', () => teardown(143))
// A closing pty sends SIGHUP; the default action would kill us without
// running teardown, orphaning Vite on its port.
process.on('SIGHUP', () => teardown(129))

console.log(`[dev-web] starting backend (data dir: ${dataDir})`)
