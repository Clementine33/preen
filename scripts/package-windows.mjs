#!/usr/bin/env node
/** Assemble the fixed Windows portable layout. */
import { spawnSync } from 'node:child_process'
import { cpSync, mkdirSync, renameSync, rmSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { releaseVersion } from './release-version.mjs'

const repository = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

export function packageWindows({ root = repository, rawTag = process.env.RAW_TAG, archive } = {}) {
  const version = releaseVersion(rawTag)
  const binaries = path.join(root, 'dist/desktop/windows-amd64')
  const tools = path.join(root, 'dist/desktop-ffmpeg')
  const files = [
    [path.join(binaries, 'preen-desktop.exe'), 'preen-desktop.exe'],
    [path.join(binaries, 'bin/preen.exe'), 'bin/preen.exe'],
    ...['ffmpeg.exe', 'ffprobe.exe'].map((name) => [path.join(tools, 'bin', name), `bin/${name}`]),
    [path.join(root, 'LICENSE'), 'LICENSE'],
    [path.join(tools, 'THIRD_PARTY_NOTICES.txt'), 'THIRD_PARTY_NOTICES.txt'],
  ]
  for (const [source] of files) {
    if (!statSync(source, { throwIfNoEntry: false })?.isFile() || statSync(source).size === 0) {
      throw new Error(`Missing or empty portable input: ${source}`)
    }
  }
  const stage = path.join(root, 'dist/portable-windows-amd64')
  const output = path.join(root, `dist/preen-${version}-windows-amd64.zip`)
  rmSync(stage, { recursive: true, force: true })
  for (const [source, destination] of files) {
    const target = path.join(stage, destination)
    mkdirSync(path.dirname(target), { recursive: true })
    cpSync(source, target)
  }
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
