#!/usr/bin/env node
/** Assemble the fixed portable layout; its corresponding sources ship as CI artifacts. */
import { spawnSync } from 'node:child_process'
import { cpSync, mkdirSync, readFileSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { releaseVersion } from './build-desktop.mjs'

const repository = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

export function requiredLicenses(root) {
  const pins = readFileSync(path.join(root, 'mise.toml'), 'utf8')
  const version = (name) => {
    const value = pins.match(new RegExp(`^${name} = "([^"]+)"$`, 'm'))?.[1]
    if (!value) throw new Error(`Missing ${name} version in mise.toml`)
    return value
  }
  return [
    'README.txt', 'ffmpeg-LGPL-2.1.txt', 'ffmpeg-LICENSE.md', 'ffmpeg-CREDITS.txt',
    'ffmpeg-configure.txt', 'ffmpeg.exe.imports.txt', 'ffprobe.exe.imports.txt',
    'lame-LGPL-2.0.txt', 'lame-LICENSE.txt', 'opus-BSD.txt', 'zlib-LICENSE.txt',
    'compiler.txt', 'mingw-w64-copyright.txt', 'gcc-runtime-copyright.txt', 'gcc-runtime-GPL-3.txt',
    ...['ffmpeg', 'lame', 'opus', 'zlib'].map((name) => `${name}-${version(name)}-copyright.txt`),
  ]
}

export function packageWindows({ root = repository, rawTag = process.env.RAW_TAG, commit = process.env.GITHUB_SHA, archive } = {}) {
  const version = releaseVersion(rawTag)
  const binaries = path.join(root, 'dist/desktop/windows-amd64')
  const tools = path.join(root, 'dist/desktop-ffmpeg')
  const files = [
    [path.join(binaries, 'preen-desktop.exe'), 'preen-desktop.exe'],
    [path.join(binaries, 'bin/preen.exe'), 'bin/preen.exe'],
    ...['ffmpeg.exe', 'ffprobe.exe'].map((name) => [path.join(tools, 'bin', name), `bin/${name}`]),
    [path.join(root, 'LICENSE'), 'licenses/Preen-GPL-3.0.txt'],
    ...requiredLicenses(root).map((name) => [path.join(tools, 'licenses', name), `licenses/${name}`]),
  ]
  for (const [source] of files) {
    if (!statSync(source, { throwIfNoEntry: false })?.isFile() || statSync(source).size === 0) {
      throw new Error(`Missing or empty portable input: ${source}`)
    }
  }
  if (!commit) {
    const result = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' })
    if (result.status !== 0) throw new Error('Cannot determine Preen source commit')
    commit = result.stdout.trim()
  }
  if (!/^[0-9a-f]{40}$/.test(commit)) throw new Error('Expected a full Preen source commit')
  const stage = path.join(root, 'dist/portable-windows-amd64')
  const output = path.join(root, `dist/preen-${version}-windows-amd64.zip`)
  rmSync(stage, { recursive: true, force: true })
  for (const [source, destination] of files) {
    const target = path.join(stage, destination)
    mkdirSync(path.dirname(target), { recursive: true })
    cpSync(source, target)
  }
  const sourceURL = `https://github.com/${process.env.GITHUB_REPOSITORY || 'Clementine33/preen'}`
  const runURL = process.env.GITHUB_RUN_ID ? `${sourceURL}/actions/runs/${process.env.GITHUB_RUN_ID}` : 'the same build\'s dist/desktop-ffmpeg/sources directory'
  writeFileSync(path.join(stage, 'licenses/Preen-README.txt'), `Preen ${version} (Windows x86_64 portable)\n\nExtract the entire archive to a writable folder and run preen-desktop.exe.\nData and logs are created in data/ beside the executable. Preserve data/ when upgrading.\nRequires Microsoft Edge WebView2 Runtime: https://developer.microsoft.com/microsoft-edge/webview2/\n\nPreen is licensed under GPLv3; see Preen-GPL-3.0.txt.\nCorresponding Preen source and build instructions: ${sourceURL}/tree/${commit}\nBuild commit: ${commit}\nThe manual workflow also uploads a Preen source archive for this commit.\nAudio tool versions, licenses, configuration and rebuild instructions: README.txt.\nCorresponding audio source artifact: ${runURL}\nEvery redistribution must provide the corresponding source artifacts alongside this zip.\n`)
  try {
    if (archive) archive(stage, `${output}.tmp`)
    else {
      const result = spawnSync('go', ['run', path.join(repository, 'scripts/zip-directory.go'), stage, `${output}.tmp`], {
        stdio: 'inherit', env: { ...process.env, GOOS: '', GOARCH: '' },
      })
      if (result.error) throw result.error
      if (result.status !== 0) throw new Error(`ZIP assembly failed (${result.status})`)
    }
    renameSync(`${output}.tmp`, output)
  } finally {
    rmSync(`${output}.tmp`, { force: true })
  }
  return output
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { console.log(packageWindows()) } catch (error) { console.error(error.message); process.exitCode = 1 }
}
