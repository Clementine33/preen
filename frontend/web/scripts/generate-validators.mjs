import { mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import Ajv2020 from 'ajv/dist/2020.js'
import standaloneCode from 'ajv/dist/standalone/index.js'

const directory = fileURLToPath(new URL('../../../docs/api/schema/', import.meta.url))
const output = process.argv[2] ?? fileURLToPath(new URL('../src/lib/api/validators.gen.js', import.meta.url))
const files = readdirSync(directory).filter(file => file.endsWith('.json') && file !== 'contracts.json').sort()
const ajv = new Ajv2020({
  strict: false,
  validateFormats: false,
  allErrors: true,
  code: { source: true, esm: true },
})
for (const file of files)
  ajv.addSchema(JSON.parse(readFileSync(join(directory, file), 'utf8')), file)
const names = Object.fromEntries(files.map(file => [
  file.slice(0, -5),
  file,
]))
mkdirSync(dirname(output), { recursive: true })
writeFileSync(output, `// Generated AJV standalone validators. Do not edit.\n${standaloneCode(ajv, names)}\n`)
writeFileSync(output.replace(/\.js$/, '.d.ts'), `// Generated validator declarations.\nimport type { ValidateFunction } from 'ajv'\nimport type { WireSchemas } from './wire.gen'\n${Object.keys(names).map(name => `export const ${name}: ValidateFunction<WireSchemas['${name}']>`).join('\n')}\n`)
