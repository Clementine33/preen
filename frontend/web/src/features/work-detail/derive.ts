// The detail page's pure derivations: everything computed from the content
// manifest (docs/api.md "Work content manifest") and raw text, independent of
// any component — which makes them the unit-test focus.

import type { WorkContent, WorkContentCopy, WorkContentDir, WorkContentFile, WorkFileKind } from '@/lib/api/types'
import type { QueueItem } from '@/stores/player'

/* ── Tree walking ── */

function* walkFiles(dir: WorkContentDir, naturalOrder = false): Generator<WorkContentFile> {
  const children = naturalOrder ? [...dir.children].sort((a, b) =>
    Number('children' in a) - Number('children' in b) || a.name.localeCompare(b.name, undefined, { numeric: true }),
  ) : dir.children
  for (const child of children) {
    if ('children' in child) yield* walkFiles(child, naturalOrder)
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
  /** Unique identity: variant + stem, so same-named files in sibling variants stay apart. */
  key: string
  /** The extension-stripped basename every format of this track shares. */
  stem: string
  /** The folder path with format markers removed ("SE有"); '' when the tracks sit at the root. */
  variant: string
  /** The file per format; tracks remain listed when the browsed format is missing. */
  files: Partial<Record<string, WorkContentFile>>
  /** Every format this track exists in, in first-seen order. */
  formats: string[]
  /** The same-basename timed lyrics (LRC or WebVTT), the automatic pairing. */
  lrcRel: string | null
  duration: number | null
}

const stemOf = (name: string): string => name.replace(/\.[^.]+$/, '')
const dirOf = (relPath: string): string => relPath.slice(0, Math.max(relPath.lastIndexOf('/'), 0))

/**
 * A folder path with the work's own format markers removed from every segment
 * ("wav_se_on" and "mp3_se_on" both become "se_on"), so the same variant of
 * different formats meets while sibling variants stay apart.
 */
// ponytail: keyword heuristic; add to the noise list when a real work misses.
function variantOf(dir: string, formats: readonly string[]): string {
  const marker = formats.length > 0
    ? new RegExp(`(?<![a-z])(${formats.map((f) => f.replace(/[^\w]/g, '\\$&')).join('|')})(?![a-z])`, 'gi')
    : null
  const noise = /\d+(\.\d+)?\s*(kbps|khz|bit)|版|形式|(?<![a-z])(ver|version|format)(?![a-z])/gi
  return dir
    .split('/')
    .map((segment) => (marker ? segment.replace(marker, '') : segment).replace(noise, '').replace(/^[\s_\-.()[\]【】]+|[\s_\-.()[\]【】]+$/g, ''))
    .filter(Boolean)
    .join('/')
}

/**
 * The one track list: audio files aggregate by variant (folder path minus
 * format markers) and extension-stripped basename, in files-first natural
 * depth-first order. A later format fills the existing track without moving
 * its first-seen position. A file whose format slot is already taken keeps its
 * raw folder as variant, so nothing is overwritten.
 */
export function deriveTracks(tree: WorkContentDir): Track[] {
  const audio: WorkContentFile[] = []
  const lrcs: WorkContentFile[] = []
  for (const file of walkFiles(tree, true)) {
    if (file.kind === 'aud') audio.push(file)
    else if (file.kind === 'lrc') lrcs.push(file)
  }
  const priority = (name: string) => /\.vtt$/i.test(name) ? 0 : 1
  lrcs.sort((a, b) => priority(a.name) - priority(b.name))
  const formats = [...new Set(audio.flatMap((f) => (f.format ? [f.format] : [])))]
  const keyOf = (variant: string, stem: string) => (variant ? `${variant}/${stem}` : stem)
  type Entry = { stem: string; variant: string; files: Partial<Record<string, WorkContentFile>>; formats: string[] }
  const byKey = new Map<string, Entry>()
  for (const f of audio) {
    const stem = stemOf(f.name)
    let variant = variantOf(dirOf(f.rel_path), formats)
    if (f.format && byKey.get(keyOf(variant, stem))?.files[f.format]) variant = dirOf(f.rel_path)
    const key = keyOf(variant, stem)
    const entry = byKey.get(key) ?? { stem, variant, files: {}, formats: [] }
    if (f.format) {
      entry.files[f.format] = f
      if (!entry.formats.includes(f.format)) entry.formats.push(f.format)
    }
    byKey.set(key, entry)
  }
  const lrcVariants = new Map(lrcs.map((lrc) => [lrc, variantOf(dirOf(lrc.rel_path), formats)]))
  return [...byKey].map(([key, entry]) => {
    const sameStem = lrcs.filter((lrc) => stemOf(lrc.name) === entry.stem)
    const lrc = sameStem.find((l) => lrcVariants.get(l) === entry.variant) ?? sameStem[0]
    return {
      key,
      stem: entry.stem,
      variant: entry.variant,
      files: entry.files,
      formats: entry.formats,
      lrcRel: lrc?.rel_path ?? null,
      duration: Object.values(entry.files).find((f) => f?.duration_sec != null)?.duration_sec ?? null,
    }
  })
}

/** The copy a work plays and reads from: the first online one, else the first. */
export function playableCopy(content: WorkContent | null): WorkContentCopy | null {
  const copies = content?.copies ?? []
  return copies.find((copy) => copy.online) ?? copies[0] ?? null
}

/**
 * The player's queue for starting at `tracks[start]`: one codec, one variant.
 * The codec is `fmt` when the start track has it, else the start track's first
 * format; the queue holds the tracks of the start track's variant that exist in
 * that codec, so continuous playback never changes codec or replays the same
 * track from a sibling variant. `index` is the start track's place in it.
 * Without an address resolver every entry plays silently.
 */
export function playQueue(
  tracks: readonly Track[],
  start: number,
  fmt: string,
  src?: (file: WorkContentFile) => string,
): { queue: QueueItem[]; index: number; format: string | null } {
  const first = tracks[start]
  if (!first) return { queue: [], index: 0, format: null }
  const format = first.files[fmt] ? fmt : (first.formats[0] ?? null)
  const members = tracks.filter((t) => t.variant === first.variant && (format === null || t.files[format]))
  const queue = members.map((track) => {
    const file = format === null ? undefined : track.files[format]
    return {
      key: track.key,
      title: track.stem,
      source: file && src ? { src: src(file), durationSec: file.duration_sec, format: file.format } : undefined,
    }
  })
  return { queue, index: Math.max(members.indexOf(first), 0), format }
}

/** Every timed-lyrics file in the same files-first natural depth-first order as audio. */
export function lrcFilesOf(tree: WorkContentDir): WorkContentFile[] {
  return [...walkFiles(tree, true)].filter((f) => f.kind === 'lrc')
}

/** The format labels a work's audio actually ships, for the browse chips. */
export function trackFormats(tracks: readonly Track[]): string[] {
  const seen: string[] = []
  for (const track of tracks) for (const format of track.formats) if (!seen.includes(format)) seen.push(format)
  const priority = (format: string) => format === 'MP3' ? 0 : format === 'WAV' ? 1 : 2
  return seen.sort((a, b) => priority(a) - priority(b))
}

/**
 * The effective format of a work: the first preferred format it ships, or null
 * (no preference, or none of them shipped) meaning every format stays merged.
 */
export function pickFormat(formats: readonly string[], preference: readonly string[]): string | null {
  return preference.find((format) => formats.includes(format)) ?? null
}

/* ── Raw text parsing ── */

export interface LrcLine {
  time: number
  /** WebVTT cues stop at this time; LRC stays until the next line. */
  endTime?: number
  text: string
  /** A full-line parenthetical is a sound effect: set smaller (v3's sfx). */
  sfx: boolean
}

/** `[mm:ss.xx]`-tagged lines; the first tag on a line wins, untagged lines skip. */
export function parseLrc(text: string): LrcLine[] {
  const lines: LrcLine[] = []
  for (const raw of text.split(/\r\n?|\n/)) {
    // One time is enough; any further tags on the line are dropped with it.
    const match = /\[(\d{1,2}):(\d{1,2}(?:\.\d+)?)\](?:\[[^\]]*\])*(.*)$/.exec(raw)
    if (!match) continue
    const seconds = Number(match[1]) * 60 + Number(match[2])
    const body = match[3].trim()
    if (body === '') continue
    lines.push({ time: seconds, text: body, sfx: body.startsWith('（') })
  }
  return lines.sort((a, b) => a.time - b.time)
}

/** Timed text for the LISTEN pane; WebVTT layout tags are displayed as plain text. */
export function parseLyrics(text: string, relPath: string): LrcLine[] {
  if (!/\.vtt$/i.test(relPath)) return parseLrc(text)
  const source = text.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  if (!/^WEBVTT(?:[ \t].*)?(?:\n|$)/.test(source)) return []
  const lines: LrcLine[] = []
  for (const block of source.split(/\n[ \t]*\n/).slice(1)) {
    const rows = block.split('\n')
    if (/^(?:NOTE(?:[ \t]|$)|STYLE$|REGION$)/.test(rows[0])) continue
    const timing = rows[0].includes('-->') ? 0 : 1
    const match = /^(\S+)[ \t]+-->[ \t]+(\S+)(?:[ \t].*)?$/.exec(rows[timing] ?? '')
    if (!match) continue
    const time = vttTime(match[1])
    const endTime = vttTime(match[2])
    if (time === null || endTime === null || endTime <= time) continue
    // Strip cue tags before decoding entities; escaped angle brackets remain text.
    const body = rows.slice(timing + 1).join('\n').replace(/<[^>]*>/g, '').replace(
      /&(amp|lt|gt|nbsp|lrm|rlm);/g,
      (_, entity: string) => ({ amp: '&', lt: '<', gt: '>', nbsp: '\u00a0', lrm: '\u200e', rlm: '\u200f' })[entity]!,
    ).trim()
    if (body) lines.push({ time, endTime, text: body, sfx: body.startsWith('（') })
  }
  return lines.sort((a, b) => a.time - b.time)
}

function vttTime(value: string): number | null {
  const match = /^(?:(\d{2,}):)?([0-5]\d):([0-5]\d)\.(\d{3})$/.exec(value)
  return match ? Number(match[1] ?? 0) * 3600 + Number(match[2]) * 60 + Number(match[3]) + Number(match[4]) / 1000 : null
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
