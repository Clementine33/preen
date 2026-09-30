// The detail page's pure derivations (draft spec §2): everything computed from
// the content manifest and raw text, independent of the seam and of any
// component. They survive the Phase C swap unchanged, so they are the unit-test
// focus: if these stay green after the swap, the swap changed nothing.

import type { WorkContentDir, WorkContentFile, WorkFileKind } from '@/lib/api/types'

/* ── Tree walking ── */

function* walkFiles(dir: WorkContentDir): Generator<WorkContentFile> {
  for (const child of dir.children) {
    if ('children' in child) yield* walkFiles(child)
    else yield child
  }
}

/** The directory a breadcrumb of names points at, or null when the path is gone. */
export function dirAt(tree: WorkContentDir, cwd: readonly string[]): WorkContentDir | null {
  let node: WorkContentDir = tree
  for (const segment of cwd) {
    const next = node.children.find((child): child is WorkContentDir => 'children' in child && child.name === segment)
    if (!next) return null
    node = next
  }
  return node
}

/** Recursive per-kind counts of a directory (lrc counts as txt). */
export function dirCounts(dir: WorkContentDir): { aud: number; img: number; txt: number } {
  const counts = { aud: 0, img: 0, txt: 0 }
  for (const child of walkFiles(dir)) {
    if (child.kind === 'aud') counts.aud++
    else if (child.kind === 'img') counts.img++
    else if (child.kind === 'txt' || child.kind === 'lrc') counts.txt++
  }
  return counts
}

/** Total files of every kind under a directory. */
export function fileCount(dir: WorkContentDir): number {
  let n = 0
  for (const _ of walkFiles(dir)) n++
  return n
}

/* ── Image sets ── */

export interface ImageSet {
  id: string
  /** The set's path from the copy root; the label chip shows the id, the title the path. */
  path: string
  /** Depth from the copy root; the cover set is the shallowest (表紙 convention). */
  depth: number
  files: WorkContentFile[]
  cover: boolean
}

/**
 * An image set is every directory that directly holds images (v3). The cover
 * set is the shallowest one — ties resolved by tree order, the order a real
 * work's folders sit in — and sorts first; the rest keep walk order.
 */
export function imageSets(tree: WorkContentDir): ImageSet[] {
  const found: ImageSet[] = []
  const walk = (dir: WorkContentDir, depth: number, path: string): void => {
    const imgs = dir.children.filter((child): child is WorkContentFile => !('children' in child) && child.kind === 'img')
    if (imgs.length > 0) found.push({ id: dir.name, path, depth, files: imgs, cover: false })
    for (const child of dir.children) {
      if ('children' in child) walk(child, depth + 1, path ? `${path}/${child.name}` : child.name)
    }
  }
  walk(tree, 0, '')
  const minDepth = Math.min(...found.map((set) => set.depth))
  const cover = found.find((set) => set.depth === minDepth)
  if (cover) cover.cover = true
  return cover ? [cover, ...found.filter((set) => set !== cover)] : found
}

/** An image taller than 2.5× its width reads as a scroll: fit-width, pan vertically (v3). */
export function isScrollShape(width: number | null, height: number | null): boolean {
  return width !== null && height !== null && height > 0 && width / height < 1 / 2.5
}

/* ── Tracks ── */

export interface Track {
  /** The extension-stripped basename every format of this track shares. */
  stem: string
  /** The file per format; a track missing the browsed format still lists. */
  files: Partial<Record<string, WorkContentFile>>
  /** Every format this track exists in, in first-seen order. */
  formats: string[]
  /** The same-basename .lrc, the automatic pairing (v3's AUTO_PAIR). */
  lrcRel: string | null
  duration: number | null
}

const stemOf = (name: string): string => name.replace(/\.[^.]+$/, '')

/**
 * The one track list: audio files aggregate by extension-stripped basename,
 * naturally ordered; nothing is invented — a missing slot is a missing file.
 */
export function deriveTracks(tree: WorkContentDir): Track[] {
  const audio: WorkContentFile[] = []
  const lrcs: WorkContentFile[] = []
  for (const f of walkFiles(tree)) {
    if (f.kind === 'aud') audio.push(f)
    else if (f.kind === 'lrc') lrcs.push(f)
  }
  const byStem = new Map<string, { files: Partial<Record<string, WorkContentFile>>; formats: string[] }>()
  for (const f of audio) {
    const stem = stemOf(f.name)
    const entry = byStem.get(stem) ?? { files: {}, formats: [] }
    if (f.format) {
      entry.files[f.format] = f
      if (!entry.formats.includes(f.format)) entry.formats.push(f.format)
    }
    byStem.set(stem, entry)
  }
  return [...byStem]
    .sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true }))
    .map(([stem, entry]) => ({
      stem,
      files: entry.files,
      formats: entry.formats,
      lrcRel: lrcs.find((lrc) => stemOf(lrc.name) === stem)?.rel_path ?? null,
      duration: Object.values(entry.files).find((f) => f?.duration_sec != null)?.duration_sec ?? null,
    }))
}

/** The format labels a work's audio actually ships, for the browse chips. */
export function trackFormats(tracks: readonly Track[]): string[] {
  const seen: string[] = []
  for (const track of tracks) for (const format of track.formats) if (!seen.includes(format)) seen.push(format)
  return seen
}

/* ── Raw text parsing ── */

export interface LrcLine {
  time: number
  text: string
  /** A full-line parenthetical is a sound effect: set smaller (v3's sfx). */
  sfx: boolean
}

/** `[mm:ss.xx]`-tagged lines; the first tag on a line wins, untagged lines skip. */
export function parseLrc(text: string): LrcLine[] {
  const lines: LrcLine[] = []
  for (const raw of text.split('\n')) {
    // One time is enough; any further tags on the line are dropped with it.
    const match = /\[(\d{1,2}):(\d{1,2}(?:\.\d+)?)\](?:\[[^\]]*\])*(.*)$/.exec(raw)
    if (!match) continue
    const seconds = Number(match[1]) * 60 + Number(match[2])
    const body = match[3].trim()
    if (body === '') continue
    lines.push({ time: seconds, text: body, sfx: body.startsWith('（') })
  }
  return lines
}

export type ScriptLine = { kind: 'scene'; text: string } | { kind: 'sfx'; text: string } | { kind: 'sp'; text: string }

/**
 * A script's only structure is its typography: 【…】 heads a scene, a full-line
 * （…） is a sound effect, everything else is numbered speech.
 */
export function parseTxt(text: string): ScriptLine[] {
  const lines: ScriptLine[] = []
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (line === '') continue
    if (line.startsWith('【') && line.endsWith('】')) lines.push({ kind: 'scene', text: line.slice(1, -1) })
    else if (line.startsWith('（') && line.endsWith('）')) lines.push({ kind: 'sfx', text: line.slice(1, -1) })
    else lines.push({ kind: 'sp', text: line })
  }
  return lines
}

/* ── Telemetry formatting ── */

/** `mm:ss`. */
export function fmtT(seconds: number): string {
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(Math.floor(seconds % 60)).padStart(2, '0')}`
}

/** `HhMMm` over an hour, else mm:ss (v3's bay summary). */
export function fmtLong(seconds: number): string {
  return seconds >= 3600
    ? `${Math.floor(seconds / 3600)}H${String(Math.floor((seconds % 3600) / 60)).padStart(2, '0')}M`
    : fmtT(seconds)
}

/** Bytes as `6.2K` / `212M`, the size column's vocabulary. */
export function fmtSize(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(1)}G`
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)}M`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)}K`
  return String(bytes)
}

/** `YYYY.MM.DD` from the catalog's `YYYY-MM-DD`, or the em-dash when unknown. */
export function dottedDate(value: string | null): string {
  return value ? value.replaceAll('-', '.') : '—'
}

/** The file-kind glyph of the browser rows (v3). */
export function kindGlyph(kind: WorkFileKind | 'dir'): string {
  return { dir: '▸', aud: '♪', img: '▢', txt: '¶', lrc: '¶', bin: '◇' }[kind]
}

/** The file-row INFO of one audio file: its duration and its format label. */
export function audioInfo(f: WorkContentFile): string {
  const label = f.format ? `${f.format}${f.bitrate ? ` ${f.bitrate}K` : ''}` : ''
  return f.duration_sec !== null ? `${fmtT(f.duration_sec)} · ${label}` : label
}
