// Guards the single type scale (DESIGN.md §6/§7): font sizes come from Tailwind's
// --text-* steps, never from arbitrary `text-[11px]` classes or literal `font-size:`.
// A line opts out with `type-scale: exception <reason>` in a comment on that line.
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { extname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const ARBITRARY = /\btext-\[\s*\.?\d[^\]]*\]/ // text-[11px]; not text-[var(--text-muted)] (colour), nor text-[length:var(--input-font)]
const LITERAL = /\bfont-size:\s*\.?\d+(?:\.\d+)?(?:px|rem)\b/
const EXCEPTION = /type-scale:\s*exception\s+\S/

export function violations(line, ext) {
  if (EXCEPTION.test(line))
    return []
  const out = []
  if ((ext === '.vue' || ext === '.ts') && ARBITRARY.test(line))
    out.push('arbitrary font size (use text-2xs/xs/sm/xl)')
  if ((ext === '.vue' || ext === '.css') && LITERAL.test(line))
    out.push('literal font-size (use var(--text-*))')
  return out
}

assert.equal(violations('class="text-[11px] x"', '.vue').length, 1)
assert.equal(violations('class="text-[0.7rem]"', '.ts').length, 1)
assert.equal(violations('class="text-[var(--text-muted)] text-[color:red]"', '.vue').length, 0)
assert.equal(violations('font-size: 15px;', '.css').length, 1)
assert.equal(violations('font-size: var(--text-xs);', '.vue').length, 0)
assert.equal(violations('font-size: 15px; /* type-scale: exception mincho numeral */', '.vue').length, 0)

function* walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory())
      yield* walk(path)
    else if (['.vue', '.ts', '.css'].includes(extname(path)))
      yield path
  }
}

const src = join(fileURLToPath(new URL('.', import.meta.url)), '..', 'src')
const failures = []
for (const file of walk(src)) {
  readFileSync(file, 'utf8').split('\n').forEach((line, i) => {
    for (const why of violations(line, extname(file)))
      failures.push(`${file}:${i + 1}: ${why}: ${line.trim().slice(0, 100)}`)
  })
}
if (failures.length) {
  console.error(`type-scale: ${failures.length} violation(s)\n${failures.join('\n')}`)
  process.exit(1)
}
