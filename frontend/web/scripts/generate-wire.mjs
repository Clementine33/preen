import { readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { compile } from 'json-schema-to-typescript'

const schemas = fileURLToPath(new URL('../../../docs/api/schema/', import.meta.url))
const names = readdirSync(schemas).filter(name => name.endsWith('.json') && name !== 'contracts.json').sort()
// Flatten document-local definitions before compilation. The candidate's v16
// resolver cannot reliably combine relative $id and external $defs references.
// This changes reference locations only; every wire constraint stays intact.
const definitions = {}
// Task kind -> generic schema name -> task-specific schema id, read from the
// generic schemas' anyOf of `<kind>_v<version>_<name>.json` refs. The web client
// binds a contract before any response arrives, so it knows one version per kind.
const taskSchemas = {}
const taskVersions = {}
function relocate(value, name) {
  if (Array.isArray(value))
    return value.map(item => relocate(item, name))
  if (value === null || typeof value !== 'object')
    return value
  return Object.fromEntries(Object.entries(value).filter(([key]) => !['$id', '$schema', '$defs'].includes(key)).map(([key, child]) => {
    if (key === '$ref') {
      const target = child.startsWith('#/$defs/')
        ? `${name}__${child.slice('#/$defs/'.length)}`
        : child.replace(/\.json$/, '')
      return [key, `#/definitions/${target}`]
    }
    return [
      key,
      relocate(child, name),
    ]
  }))
}
for (const file of names) {
  const name = file.slice(0, -5)
  const schema = JSON.parse(readFileSync(`${schemas}${file}`, 'utf8'))
  definitions[name] = relocate(schema, name)
  for (const branch of schema.anyOf ?? []) {
    const match = /^(.+)_v(\d+)_(.+)\.json$/.exec(branch.$ref ?? '')
    if (match?.[3] !== name)
      continue
    const [, kind, version] = match
    taskVersions[kind] ??= version
    if (taskVersions[kind] !== version) {
      throw new Error(`Task ${kind} has schema versions ${taskVersions[kind]} and ${version}; the web client binds one version per kind`)
    }
    taskSchemas[kind] ??= {}
    taskSchemas[kind][name] = branch.$ref.slice(0, -5)
  }
  for (const [key, value] of Object.entries(schema.$defs ?? {}))
    definitions[`${name}__${key}`] = relocate(value, name)
}
const properties = Object.fromEntries(names.map(file => [
  file.slice(0, -5),
  { $ref: `#/definitions/${file.slice(0, -5)}` },
]))
let code = await compile({
  type: 'object',
  definitions,
  properties,
  required: Object.keys(properties),
  additionalProperties: false,
}, 'WireSchemas', {
  cwd: schemas,
  bannerComment: '// Generated from Go wire schemas. Do not edit; run task contract:generate.',
  unknownAny: true,
  style: { singleQuote: true, semi: false, trailingComma: 'all' },
})
const bindings = JSON.parse(readFileSync(`${schemas}contracts.json`, 'utf8'))
for (const [family, events] of Object.entries(bindings.sse)) {
  const name = `Wire${family[0].toUpperCase()}${family.slice(1)}Event`
  code += `\nexport type ${name} = ${Object.entries(events).map(([event, schema]) => `{ type: '${event}'; data: WireSchemas['${schema}'] }`).join(' | ')}\n`
}
const output = process.argv[2] ?? fileURLToPath(new URL('../src/lib/api/wire.gen.ts', import.meta.url))
writeFileSync(output, code)

// The draft request is not a generic/bound pair; the Go side names it per Task.
for (const [kind, request] of Object.entries(bindings.task_draft_requests)) {
  if (!taskSchemas[kind] || !names.includes(`${request}.json`))
    throw new Error(`Task ${kind} draft request ${request} has no schema`)
  taskSchemas[kind].draftDocumentRequest = request
}
const sortedTaskSchemas = Object.fromEntries(Object.entries(taskSchemas).sort(([a], [b]) => a.localeCompare(b)))

const contracts = `// Generated HTTP status and SSE event bindings. Do not edit.\nimport type { WireSchemas } from './wire.gen'\nexport type SchemaId = keyof WireSchemas\nexport const schemaIds = ${JSON.stringify(names.map(file => file.slice(0, -5)))} as const satisfies readonly SchemaId[]\nexport const httpContracts = ${JSON.stringify(bindings.http, null, 2)} as const satisfies Record<string, { request: SchemaId | ''; responses: Record<string, SchemaId | '@empty' | '@sse'> }>\nexport const sseContracts = ${JSON.stringify(bindings.sse, null, 2)} as const satisfies Record<string, Record<string, SchemaId>>\nexport const taskSchemas = ${JSON.stringify(sortedTaskSchemas, null, 2)} as const satisfies Record<string, Record<string, SchemaId>>\n`
writeFileSync(join(dirname(output), 'contracts.gen.ts'), contracts)
