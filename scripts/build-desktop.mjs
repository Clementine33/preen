#!/usr/bin/env node
/** Build the shared web assets and both desktop binaries in their installed layout. */
import { spawn, spawnSync } from 'node:child_process'
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

export function releaseVersion(raw) {
  const version = raw?.replace(/^v/, '') ?? ''
  const number = '(0|[1-9][0-9]*)'
  const identifier = '(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
  const semver = new RegExp(`^${number}\\.${number}\\.${number}(?:-${identifier}(?:\\.${identifier})*)?(?:\\+[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?$`)
  const prerelease = version.split('+')[0].split('-').slice(1).join('-')
  if (!semver.test(version) || prerelease.split('.').includes('dev')) {
    throw new Error('RAW_TAG must be an explicit SemVer release (for example v1.2.3), without a dev prerelease identifier')
  }
  return version
}

function run(command, args, cwd = root, env = process.env) {
  const result = spawnSync(command, args, { cwd, env, stdio: 'inherit', shell: process.platform === 'win32' && command === 'pnpm' })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`${command} failed (${result.status ?? result.signal})`)
}

function build() {
  const dev = process.argv.includes('--dev')
  const version = dev
    ? readFileSync(path.join(root, 'backend/go/cmd/preen/main.go'), 'utf8').match(/var version = "([^"]+)"/)[1]
    : releaseVersion(process.env.RAW_TAG)
  const goos = process.env.GOOS || ({ win32: 'windows', linux: 'linux' })[process.platform]
  const goarch = process.env.GOARCH || ({ x64: 'amd64', arm64: 'arm64' })[process.arch]
  if (!['linux', 'windows'].includes(goos) || !goarch) throw new Error('Desktop builds support Linux and Windows')
  const output = path.join(root, 'dist', dev ? 'desktop-dev' : 'desktop', `${goos}-${goarch}`)
  const shell = path.join(output, goos === 'windows' ? 'preen-desktop.exe' : 'bin/preen-desktop')
  const backend = path.join(output, goos === 'windows' ? 'bin/preen.exe' : 'lib/preen/preen')
  // Development serves the web app from Vite through Wails, so edits reload in place.
  const webDir = path.join(root, 'frontend/web')
  const vite = dev && spawn(process.execPath, [path.join(webDir, 'node_modules/vite/bin/vite.js'), '--host', '127.0.0.1', '--port', '5174', '--strictPort'], { cwd: webDir, env: { ...process.env, PREEN_VUE_DEVTOOLS: '0' }, stdio: 'inherit' })
  process.on('exit', () => vite && vite.kill())
  if (!dev) {
    run('pnpm', ['--dir', webDir, 'build'])
    const assets = path.join(root, 'desktop/assets')
    rmSync(assets, { recursive: true, force: true })
    cpSync(path.join(root, 'frontend/web/dist'), assets, { recursive: true })
    writeFileSync(path.join(assets, '.gitkeep'), '')
  }
  rmSync(output, { recursive: true, force: true })
  mkdirSync(path.dirname(backend), { recursive: true })
  mkdirSync(path.dirname(shell), { recursive: true })
  const env = { ...process.env, GOOS: goos, GOARCH: goarch }
  run('go', ['build', '-trimpath', '-ldflags', `-X main.version=${version}`, '-o', backend, './cmd/preen'], path.join(root, 'backend/go'), env)
  run('go', ['build', '-trimpath', ...(dev ? [] : ['-tags', 'production']), '-ldflags', `${goos === 'windows' ? '-H windowsgui ' : ''}-X main.version=${version}`, '-o', shell, '.'], path.join(root, 'desktop'), env)
  console.log(`Desktop ${version}: ${output}`)
  if (dev) {
    run(shell, [], root, { ...process.env, XDG_DATA_HOME: path.join(root, '.dev_data/desktop'), FRONTEND_DEVSERVER_URL: 'http://127.0.0.1:5174' })
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { build() } catch (error) { console.error(error.message); process.exitCode = 1 }
}
