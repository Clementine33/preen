import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'

// Only creates resources named with this invocation's random prefix.
const image = process.argv[2]
assert(image, 'Usage: node scripts/docker-smoke.mjs <image>')
const engine = process.env.CONTAINER_ENGINE || 'docker'
const prefix = `preen-smoke-${randomBytes(6).toString('hex')}`
const token = randomBytes(32).toString('hex')
const containers = []
const volumes = []
const scratch = mkdtempSync(join(tmpdir(), 'preen-smoke-'))
function command(args, env = {}) {
  const result = spawnSync(engine, args, { encoding: 'utf8', env: { ...process.env, ...env }, timeout: 180_000 })
  assert.equal(result.status, 0, `${engine} ${args[0]} failed: ${result.stderr}`)
  return (args[0] === 'logs' ? result.stdout + result.stderr : result.stdout).trim()
}
function volume(suffix) {
  const name = `${prefix}-${suffix}`
  command(['volume', 'create', name])
  volumes.push(name)
  return name
}
function create(suffix, data, extra = [], authenticated = true) {
  const name = `${prefix}-${suffix}`
  const args = ['create', '--name', name, '--read-only', '--user', '1000:1000', '--cap-drop=ALL',
    '--security-opt=no-new-privileges', '--tmpfs', '/tmp:rw,nosuid,nodev,size=536870912,mode=1777',
    '-v', `${data}:/data`, '-p', '127.0.0.1::8765', ...extra]
  if (authenticated) args.push('-e', 'PREEN_TOKEN')
  command([...args, image], authenticated ? { PREEN_TOKEN: token } : {})
  containers.push(name)
  return name
}
function init(data, media) {
  command(['run', '--rm', '--user', '0:0', '-v', `${data}:/data`, '-v', `${media}:/media/audio`, image,
    'sh', '-ec', 'chown 1000:1000 /data /media/audio'])
}
const pause = () => new Promise(resolve => setTimeout(resolve, 200))
async function ready(name) {
  command(['start', name])
  const port = command(['port', name, '8765/tcp']).split(':').at(-1)
  const base = `http://127.0.0.1:${port}`
  for (let i = 0; i < 150; i++) {
    try { if ((await fetch(`${base}/api/v1/health`)).ok) return base } catch {}
    await pause()
  }
  assert.fail(`container did not become ready: ${command(['logs', name]).replaceAll(token, '[redacted]')}`)
}
async function api(base, path, body, method = 'GET', headers = {}) {
  const response = await fetch(`${base}/api/v1${path}`, {
    method, headers: { Authorization: `Bearer ${token}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...headers },
    body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(120_000),
  })
  const text = await response.text()
  assert(response.ok, `${method} ${path}: ${response.status} ${text}`)
  return response.headers.get('content-type')?.includes('application/json') ? JSON.parse(text) : text
}
function stop(name) {
  command(['stop', '--time', '10', name])
  assert.equal(command(['inspect', '--format', '{{.State.ExitCode}}', name]), '0', 'SIGTERM must exit without forced kill')
  assert(!command(['logs', name]).includes(token), 'logs must not contain the key')
}
async function terminal(read, success) {
  for (let i = 0; i < 300; i++) {
    const result = await read()
    if (result.status === success) return result
    assert(!['failed', 'canceled', 'interrupted'].includes(result.status), JSON.stringify(result))
    await pause()
  }
  assert.fail('operation did not finish')
}
function checkMediaFormats(name) {
  const fixture = (args) => {
    const result = spawnSync('ffmpeg', ['-nostdin', '-v', 'error', ...args], { encoding: 'utf8', timeout: 30_000 })
    assert.equal(result.status, 0, `host FFmpeg fixture generation failed: ${result.error || result.stderr}`)
  }
  const wav = join(scratch, 'source.wav'), cover = join(scratch, 'cover.png'), jpeg = join(scratch, 'cover.jpg')
  fixture(['-f', 'lavfi', '-i', 'sine=frequency=440:duration=1', '-ac', '2', wav])
  for (const path of [cover, jpeg]) fixture(['-f', 'lavfi', '-i', 'color=red:s=32x32', '-frames:v', '1', path])
  const targets = [
    { ext: 'wav', encoder: 'pcm_s16le', muxer: 'wav', codec: 'pcm_s16le' },
    { ext: 'flac', encoder: 'flac', muxer: 'flac', codec: 'flac' },
    { ext: 'mp3', encoder: 'libmp3lame', muxer: 'mp3', codec: 'mp3' },
    { ext: 'm4a', encoder: 'aac', muxer: 'ipod', codec: 'aac' },
    { ext: 'opus', encoder: 'libopus', muxer: 'opus', codec: 'opus' },
  ]
  command(['exec', name, 'mkdir', '-p', '/tmp/formats'])
  for (const source of [...targets, { ext: 'aac', encoder: 'aac', muxer: 'adts', codec: 'aac' }]) {
    const path = join(scratch, `source.${source.ext}`)
    const picture = source.ext === 'mp3' ? jpeg : source.ext === 'm4a' ? cover : null
    if (source.ext !== 'wav') fixture(['-i', wav, ...(picture ? ['-i', picture, '-map', '0:a:0', '-map', '1:v:0', '-c:v', 'copy', '-disposition:v', 'attached_pic'] : []),
      '-c:a', source.encoder, '-f', source.muxer, path])
    const input = `/tmp/formats/source.${source.ext}`
    command(['cp', path, `${name}:${input}`])
    const probed = JSON.parse(command(['exec', name, 'sh', '-ec', 'ffprobe -v error -show_streams -of json -fd 0 -i fd: < "$1"', 'sh', input]))
    assert.equal(probed.streams.find(stream => stream.codec_type === 'audio')?.codec_name, source.codec, `probe ${source.ext}`)
    if (picture) assert(probed.streams.some(stream => stream.disposition?.attached_pic === 1), `${source.ext} cover stream`)
    for (const target of targets) {
      const output = `/tmp/formats/output.${target.ext}`
      const bitrate = ['mp3', 'm4a', 'opus'].includes(target.ext) ? '-b:a 192k' : ''
      const result = JSON.parse(command(['exec', name, 'sh', '-ec', `
        ffmpeg -nostdin -v error -xerror -fd 0 -i fd: -map 0:a:0 -map_metadata 0 -map_chapters -1 -vn \
          -c:a ${target.encoder} ${bitrate} -f ${target.muxer} -fd 1 fd: < "$1" > "$2"
        ffmpeg -nostdin -v error -xerror -fd 0 -i fd: -map 0:a:0 -f null - < "$2"
        ffprobe -v error -show_streams -of json -fd 0 -i fd: < "$2"
      `, 'sh', input, output]))
      assert.equal(result.streams.length, 1, `${source.ext} → ${target.ext}: audio only`)
      assert.equal(result.streams[0].codec_name, target.codec, `${source.ext} → ${target.ext}`)
      assert.equal(result.streams[0].channels, 2)
      assert(Number(result.streams[0].duration) > 0)
    }
  }
  return { wav, cover }
}
try {
  const data = volume('data'), media = volume('media')
  init(data, media)
  let name = create('first', data, ['-v', `${media}:/media/audio`])
  let base = await ready(name)
  assert.match(await (await fetch(`${base}/login`)).text(), /<html/i)
  assert.equal((await fetch(`${base}/api/v1/libraries`)).status, 401)
  const login = await fetch(`${base}/api/v1/session`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token }) })
  assert.equal(login.status, 204)
  assert.equal((await fetch(`${base}/api/v1/libraries`, { headers: { Cookie: login.headers.get('set-cookie').split(';')[0] } })).status, 200)
  const version = (await api(base, '/health')).version
  assert.match(version, /^\d+\.\d+\.\d+/)
  assert(!version.includes('-') || !version.slice(version.indexOf('-') + 1).split('.').includes('dev'))
  const config = JSON.parse(command(['exec', name, 'cat', '/data/config.json']))
  const template = JSON.parse(readFileSync(new URL('../backend/go/internal/adapters/settings/config.json.template', import.meta.url), 'utf8'))
  assert.deepEqual(config, template, 'first boot must deliver the shared configuration template')
  const lockedFFmpeg = readFileSync(new URL('../mise.toml', import.meta.url), 'utf8').match(/^ffmpeg = "([^"]+)"/m)[1]
  for (const tool of ['ffmpeg', 'ffprobe']) assert(command(['exec', name, tool, '-version']).startsWith(`${tool} version ${lockedFFmpeg}`))
  command(['exec', name, 'sh', '-ec', `
    test "$(id -u)" = 1000
    test -z "$(find /opt/ffmpeg -name '*.so*' -print)"
    ! touch /app/read-only-check
    ! command -v go
    ! command -v node
    mkdir -p '/media/audio/RJ000001 smoke/sub'
    printf move-me > '/media/audio/RJ000001 smoke/note.txt'
  `])
  const fixtures = checkMediaFormats(name)
  command(['cp', fixtures.wav, `${name}:/media/audio/RJ000001 smoke/00.wav`])
  command(['cp', fixtures.cover, `${name}:/media/audio/RJ000001 smoke/cover.png`])
  const lib = await api(base, '/libraries', { name: 'Container smoke', root_path: '/media/audio' }, 'POST')
  const scan = await api(base, `/libraries/${lib.id}/scans`, {}, 'POST')
  assert.match(scan, /event: completed/)
  assert(!scan.includes('event: error'))
  const content = await api(base, '/works/RJ00000001/content')
  const copy = content.copies[0]
  const cover = copy.tree.children.find(file => file.name === 'cover.png')
  assert(cover.thumb_v)
  const thumb = await fetch(`${base}/api/v1/works/RJ00000001/thumbs/${copy.copy_id}/cover.png?h=40&v=${cover.thumb_v}`, { headers: { Authorization: `Bearer ${token}` } })
  assert.equal(thumb.status, 200)
  assert((await thumb.arrayBuffer()).byteLength > 0)
  const op = `/libraries/${lib.id}/file-operations`
  assert.equal((await api(base, op, { member_path: 'RJ000001 smoke', operation: 'move', items: [{ source: 'note.txt', target_dir: 'sub' }] }, 'POST')).succeeded, 1)
  const deleted = await api(base, op, { member_path: 'RJ000001 smoke', operation: 'soft_delete', items: [{ source: 'sub/note.txt' }] }, 'POST')
  assert.equal(deleted.succeeded, 1)
  assert.equal(deleted.items[0].recovered_path, 'Delete/RJ000001 smoke/sub/note.txt')
  command(['exec', name, 'test', '-f', '/media/audio/Delete/RJ000001 smoke/sub/note.txt'])
  const record = await api(base, `/libraries/${lib.id}/operations/conversion/current`, { folder_paths: ['RJ000001 smoke'] }, 'PUT', { 'Idempotency-Key': 'smoke-record' })
  const operation = `/worksets/${record.workset.workset_id}/operations/conversion`
  const draft = await api(base, `${operation}/draft`)
  const generation = await api(base, `${operation}/revisions`, undefined, 'POST', { 'If-Match': String(draft.version), 'Idempotency-Key': 'smoke-plan' })
  const planned = await terminal(() => api(base, `${operation}/planning-sessions/${generation.generation.generation_id}`), 'completed')
  const current = await api(base, operation)
  const execution = await api(base, `${operation}/revisions/${planned.revision_id}/executions`, {}, 'POST', { 'If-Match': String(current.version), 'Idempotency-Key': 'smoke-execute' })
  await terminal(() => api(base, `${operation}/executions/${execution.execution.execution_id}`), 'succeeded')
  command(['exec', name, 'test', '-s', '/media/audio/RJ000001 smoke/00.mp3'])
  stop(name)
  name = create('recreated', data, ['-v', `${media}:/media/audio`])
  base = await ready(name)
  assert.equal((await api(base, '/libraries')).libraries[0].id, lib.id)
  assert.equal((await api(base, '/works/RJ00000001/content')).copies[0].copy_id, copy.copy_id)
  stop(name)
  const empty = volume('no-key')
  init(empty, media)
  const missing = create('no-key', empty, [], false)
  command(['start', missing])
  assert.notEqual(command(['wait', missing]), '0', 'LAN startup without a key must fail')
  assert(!command(['logs', missing]).includes('PREEN_BACKEND_READY'))
  // A real prior release database exercises backup and fail-closed startup in the container.
  for (const scenario of ['upgrade', 'backup-failure', 'migration-failure']) {
    const path = join(scratch, `${scenario}.db`)
    const db = new DatabaseSync(path)
    db.exec(readFileSync(new URL('../backend/go/internal/adapters/sqlite/migrations/00001_initial.sql', import.meta.url), 'utf8'))
    db.exec(`CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY AUTOINCREMENT, version_id INTEGER NOT NULL, is_applied INTEGER NOT NULL, tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
      INSERT INTO goose_db_version(version_id,is_applied) VALUES (0,1),(1,1);`)
    if (scenario === 'migration-failure') db.exec('CREATE TABLE cover_candidates (conflict TEXT)')
    db.close()
    const migrationData = volume(scenario)
    init(migrationData, media)
    const container = create(scenario, migrationData)
    command(['cp', path, `${container}:/data/cache.db`])
    command(['run', '--rm', '--user', '0:0', '-v', `${migrationData}:/data`, image, 'sh', '-ec',
      `chown 1000:1000 /data/cache.db${scenario === 'backup-failure' ? '; mkdir /data/backups; chmod 500 /data/backups; chown 1000:1000 /data/backups' : ''}`])
    if (scenario === 'upgrade') {
      const upgraded = await ready(container)
      assert.deepEqual((await api(upgraded, '/libraries')).libraries, [])
      assert.match(command(['exec', container, 'sh', '-ec', 'ls /data/backups/cache.db-*.db']), /cache.db-/)
      stop(container)
    } else {
      command(['start', container])
      assert.notEqual(command(['wait', container]), '0')
      const logs = command(['logs', container])
      assert(!logs.includes('PREEN_BACKEND_READY'))
      assert.match(logs, scenario === 'backup-failure' ? /backup before migration/ : /migrate .* to/)
      assert(!logs.includes(token))
    }
  }
  console.log('Container smoke passed: login, read-only UID1000, pinned media tools, six audio inputs × five fd outputs (including covers), scan, thumbnail, conversion, move, recovery, persistence, missing key, release migration/backup failures, SIGTERM.')
} finally {
  for (const name of containers.reverse()) command(['rm', '-f', name])
  for (const name of volumes.reverse()) command(['volume', 'rm', name])
  rmSync(scratch, { recursive: true, force: true })
}
