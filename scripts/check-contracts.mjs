import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const registry = `${root}backend/go/internal/adapters/httpapi/wire_roots_generated_test.go`
function snapshot(directory) {
  return new Map([
    ...readdirSync(directory).filter(name => name.endsWith('.json')).sort()
      .map(name => [name, readFileSync(join(directory, name), 'utf8')]),
    ['wire_roots_generated_test.go', readFileSync(registry, 'utf8')],
  ])
}
const before = snapshot(`${root}docs/api/schema`)
const temporary = mkdtempSync(join(tmpdir(), 'preen-contracts-'))
let status = 0
try {
  const result = spawnSync('go', ['generate', './internal/adapters/httpapi'], {
    cwd: `${root}backend/go`,
    env: { ...process.env, PREEN_SCHEMA_DIR: temporary },
    stdio: 'inherit',
  })
  if (result.status !== 0) {
    status = result.status ?? 1
  }
  else {
    const after = snapshot(temporary)
    const generatedWire = join(temporary, 'wire.gen.ts')
    const frontend = spawnSync('node', ['scripts/generate-wire.mjs', generatedWire], {
      cwd: `${root}frontend/web`, stdio: 'inherit',
    })
    if (frontend.status !== 0)
      throw new Error(`Wire TypeScript generation failed: ${frontend.status}`)
    for (const file of ['wire.gen.ts', 'contracts.gen.ts']) {
      before.set(file, readFileSync(`${root}frontend/web/src/lib/api/${file}`, 'utf8'))
      after.set(file, readFileSync(join(temporary, file), 'utf8'))
    }
    const changed = [...new Set([...before.keys(), ...after.keys()])].filter(name => before.get(name) !== after.get(name))
    if (changed.length) {
      console.error(`Wire contract generation differs:\n${changed.join('\n')}`)
      status = 1
    }
    else {
      console.log(`Wire contracts unchanged (${after.size - 3} schemas and bindings, generated TypeScript).`)
    }
  }
}
finally {
  rmSync(temporary, { recursive: true, force: true })
}
process.exitCode = status
