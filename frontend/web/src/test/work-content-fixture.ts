// The sample work every detail-page test binds: v3's realistically messy
// directory, in the exact shape of GET /works/{rj}/content (lib/api/types.ts
// WorkContent*) — 24 mixed-ratio CGs with one ultra-tall strip, a three-image
// cover set, eight tracks in MP3/WAV with 08 WAV-only, two LRCs, scripts, and
// a zip. Any rj answers the same sample, echoed back.

import type { WorkContent, WorkContentDir, WorkContentFile, WorkFileKind } from '@/lib/api/types'

/* ═══ The sample work: one realistically messy directory ═══ */

const TRACKS: Array<[name: string, dur: number]> = [
  ['01_導入 ―縁側にて―', 252],
  ['02_本編 ―雨音と耳かき―', 2320],
  ['03_本編 ―添い寝―', 1690],
  ['04_おやすみ', 540],
  ['05_おまけ ―晴れた朝―', 920],
  ['06_環境音 雨のみ', 3600],
  ['07_フリートーク', 610],
  ['08_後日談', 480],
]

function file(relPath: string, kind: WorkFileKind, extra: Partial<WorkContentFile> = {}): WorkContentFile {
  return {
    name: relPath.slice(relPath.lastIndexOf('/') + 1),
    rel_path: relPath,
    kind,
    size: 0,
    format: null,
    bitrate: null,
    width: null,
    height: null,
    duration_sec: null,
    thumb_v: null,
    ...extra,
  }
}

function image(relPath: string, w: number, h: number): WorkContentFile {
  return file(relPath, 'img', {
    size: Math.round(w * h * 1.4),
    width: w,
    height: h,
    thumb_v: 'mock',
  })
}

function dir(relPath: string, children: Array<WorkContentDir | WorkContentFile>): WorkContentDir {
  return {
    name: relPath === ''
      ? ''
      : relPath.slice(relPath.lastIndexOf('/') + 1),
    rel_path: relPath,
    children,
  }
}

// 24 CGs of mixed ratios, one ultra-tall scroll, with 差分 variants (v3).
const CG_SHAPES: Array<[w: number, h: number]> = [
  [1200, 1800],
  [1920, 1080],
  [1400, 1400],
  [1200, 1600],
  [1080, 1920],
  [1600, 1200],
  [1200, 1800],
  [1920, 1080],
  [1200, 1800],
  [1500, 1500],
  [1920, 1080],
  [1200, 1700],
  [1080, 1920],
  [1600, 1200],
  [1200, 1800],
  [1920, 1080],
  [1400, 1400],
  [1200, 1800],
  [1920, 1080],
  [1200, 1600],
  [1600, 1200],
  [1080, 1920],
  [1200, 1800],
  [800, 2400],
]
const cgMain: WorkContentFile[] = []
const cgDiff: WorkContentFile[] = []
CG_SHAPES.forEach(([w, h], i) => {
  const diff = i % 3 === 1 && i !== 23
  const name = i === 23
    ? 'cg_24_縦長スクロール.png'
    : `cg_${String(i + 1).padStart(2, '0')}${diff ? '_差分' : ''}.png`
  const item = image(`CG/${diff ? '差分' : '本編'}/${name}`, w, h)
  ;(diff ? cgDiff : cgMain).push(item)
})

const cover = [
  image('表紙/cover_main.jpg', 560, 420),
  image('表紙/cover_sq.jpg', 1200, 1200),
  image('表紙/帯_banner.png', 1800, 600),
]
const bonusImgs = [
  image('特典/壁紙_PC.png', 1920, 1080),
  image('特典/壁紙_SP.png', 1080, 2340),
]

// One track often ships in several formats; 08 exists only as WAV (v3).
function audio(fmt: 'MP3' | 'WAV', track: [name: string, dur: number]): WorkContentFile {
  const bytesPerSec = fmt === 'MP3'
    ? 40_000
    : 178_000
  return file(`本編/${fmt.toLowerCase()}/${track[0]}.${fmt.toLowerCase()}`, 'aud', {
    size: Math.round(track[1] * bytesPerSec),
    format: fmt,
    bitrate: fmt === 'MP3'
      ? 320
      : null,
    duration_sec: track[1],
  })
}

/* ═══ The tree ═══ */

const TREE: WorkContentDir = dir('', [
  dir('本編', [
    dir('本編/mp3', TRACKS.filter((_, t) => t < 7).map(track => audio('MP3', track))),
    dir('本編/wav', TRACKS.map(track => audio('WAV', track))),
    dir('本編/lyrics', [
      file('本編/lyrics/02_本編 ―雨音と耳かき―.lrc', 'lrc', { size: 6_348 }),
      file('本編/lyrics/04_おやすみ.lrc', 'lrc', { size: 1_843 }),
    ]),
  ]),
  dir('台本', [
    file('台本/台本_全編.txt', 'txt', { size: 90_112 }),
    file('台本/台本_おまけ.txt', 'txt', { size: 14_336 }),
  ]),
  dir('CG', [
    dir('CG/本編', cgMain),
    dir('CG/差分', cgDiff),
  ]),
  dir('表紙', cover),
  dir('特典', [
    ...bonusImgs,
    file('特典/特典音声.zip', 'bin', { size: 222_298_112 }),
    file('特典/readme.txt', 'txt', { size: 2_048 }),
  ]),
  file('はじめにお読みください.txt', 'txt', { size: 3_072 }),
])

export function sampleWorkContent(rj: string): WorkContent {
  return {
    rj,
    copies: [
      {
        copy_id: 'copy-a',
        library_id: 'lib-a',
        dir_id: 'dir-a',
        online: true,
        tree: TREE,
      },
      {
        copy_id: 'copy-b',
        library_id: 'lib-b',
        dir_id: 'dir-b',
        online: false,
      },
    ],
  }
}
