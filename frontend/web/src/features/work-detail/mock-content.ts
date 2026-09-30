// The content seam's Phase A implementation (draft spec §1): the v3 sample
// work, in the exact shape of the WorkContent contract. Deleted in Phase C.
//
// Hard constraints (draft spec §1): LRC/TXT bodies are raw text — real
// `[mm:ss.xx]` tags, real 【scene】/（sfx） script lines — so the parsers and
// renderers downstream run the real path, never a pre-structured shortcut.

import type { WorkContent, WorkContentDir, WorkContentFile, WorkFileKind } from '@/lib/api/types'
import type { ContentFileRef } from './content-source'

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

/** v3's demo-art generator: gradient + composition lines + size caption. */
function art(w: number, h: number, hue: number, label: string): string {
  const m = Math.min(w, h)
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
<defs><linearGradient id="g" x2=".7" y2="1"><stop stop-color="hsl(${hue} 32% 22%)"/><stop offset="1" stop-color="hsl(${(hue + 40) % 360} 38% 46%)"/></linearGradient></defs>
<rect width="${w}" height="${h}" fill="url(#g)"/>
<circle cx="${w * 0.62}" cy="${h * 0.38}" r="${m * 0.22}" fill="hsl(${(hue + 20) % 360} 60% 78%)" opacity=".55"/>
<path d="M0 ${h * 0.72} Q ${w * 0.4} ${h * 0.58} ${w} ${h * 0.8} V ${h} H 0 Z" fill="hsl(${hue} 30% 12%)" opacity=".55"/>
${Array.from({ length: 9 }, (_, i) => `<path d="M${(i + 1) * w / 10} 0 l ${-m * 0.12} ${h}" stroke="hsl(${hue} 50% 88%)" stroke-width="1" opacity=".12"/>`).join('')}
<text x="${m * 0.06}" y="${h - m * 0.06}" font-family="monospace" font-size="${m * 0.05}" fill="#fff" opacity=".7" letter-spacing="2">${label} · ${w}×${h}</text>
</svg>`
  return 'data:image/svg+xml,' + encodeURIComponent(svg)
}

/** The mock's pixel source: image bytes exist only as data URIs, keyed by rel_path. */
const PIXELS = new Map<string, string>()

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
    ...extra,
  }
}

function image(relPath: string, w: number, h: number, hue: number, label: string): WorkContentFile {
  PIXELS.set(relPath, art(w, h, hue, label))
  return file(relPath, 'img', { size: Math.round(w * h * 1.4), width: w, height: h })
}

function dir(relPath: string, children: Array<WorkContentDir | WorkContentFile>): WorkContentDir {
  return { name: relPath === '' ? '' : relPath.slice(relPath.lastIndexOf('/') + 1), rel_path: relPath, children }
}

// 24 CGs of mixed ratios, one ultra-tall scroll, with 差分 variants (v3).
const CG_SHAPES: Array<[w: number, h: number]> = [
  [1200, 1800], [1920, 1080], [1400, 1400], [1200, 1600], [1080, 1920], [1600, 1200], [1200, 1800], [1920, 1080],
  [1200, 1800], [1500, 1500], [1920, 1080], [1200, 1700], [1080, 1920], [1600, 1200], [1200, 1800], [1920, 1080],
  [1400, 1400], [1200, 1800], [1920, 1080], [1200, 1600], [1600, 1200], [1080, 1920], [1200, 1800], [800, 2400],
]
const cgMain: WorkContentFile[] = []
const cgDiff: WorkContentFile[] = []
CG_SHAPES.forEach(([w, h], i) => {
  const diff = i % 3 === 1 && i !== 23
  const name = i === 23 ? 'cg_24_縦長スクロール.png' : `cg_${String(i + 1).padStart(2, '0')}${diff ? '_差分' : ''}.png`
  const item = image(`CG/${diff ? '差分' : '本編'}/${name}`, w, h, (i * 23 + 190) % 360, `CG ${String(i + 1).padStart(2, '0')}`)
  ;(diff ? cgDiff : cgMain).push(item)
})

const cover = [
  image('表紙/cover_main.jpg', 560, 420, 205, 'COVER'),
  image('表紙/cover_sq.jpg', 1200, 1200, 205, 'COVER SQ'),
  image('表紙/帯_banner.png', 1800, 600, 20, 'OBI'),
]
const bonusImgs = [
  image('特典/壁紙_PC.png', 1920, 1080, 250, 'WALL PC'),
  image('特典/壁紙_SP.png', 1080, 2340, 280, 'WALL SP'),
]

// One track often ships in several formats; 08 exists only as WAV (v3).
function audio(fmt: 'MP3' | 'WAV', track: [name: string, dur: number]): WorkContentFile {
  const bytesPerSec = fmt === 'MP3' ? 40_000 : 178_000
  return file(`本編/${fmt.toLowerCase()}/${track[0]}.${fmt.toLowerCase()}`, 'aud', {
    size: Math.round(track[1] * bytesPerSec),
    format: fmt,
    bitrate: fmt === 'MP3' ? 320 : null,
    duration_sec: track[1],
  })
}

/* ═══ Raw text bodies ═══ */

const LRC_02 = `[00:00.00]（雨の音。遠くで雷が鳴る）
[00:09.00]あ、降ってきたね。
[00:15.00]こっち、縁側まで入って？ 濡れちゃうよ。
[00:24.00]……ふふ、髪、ちょっと湿ってる。
[00:33.00]（タオルで髪を拭く音）
[00:41.00]じっとしててね。耳、見せて。
[00:50.00]うん……今日はこっちから、ゆっくりいくね。
[01:00.00]（耳かきの音、左から）
[01:11.00]痛くない？ ……よかった。
[01:20.00]雨の日ってさ、世界がちょっとだけ小さくなる感じ、しない？
[01:32.00]聞こえるのは、雨と、わたしの声だけ。
[01:44.00]（梵天の音）
[01:52.00]はい、反対側。ごろんして。
[02:01.00]……眠くなってきた？ いいよ、寝ちゃっても。
[02:13.00]ちゃんと、ここにいるから。
[02:25.00]（雨音が少しずつ弱まる）`

const LRC_04 = `[00:00.00]（虫の声。雨上がりの夜）
[00:08.00]もう、雨やんだみたい。
[00:17.00]……起きてる？ ふふ、寝てるか。
[00:29.00]おやすみ。いい夢、見てね。
[00:41.00]（布団をかけ直す音）
[00:52.00]また、雨の日に。`

/** Scene markers and full-line parentheticals are the script's only structure. */
const SCRIPT_SOURCE = `【SCENE 01 · 縁側】
（雨音。遠雷。）
あ、降ってきたね。こっち、縁側まで入って？　濡れちゃうよ。
……ふふ、髪、ちょっと湿ってる。タオル持ってくるから、そこ座ってて。
（足音、遠ざかる。戻ってくる。）
はい、じっとしてて。……うん、これでよし。風邪ひいたら、わたしのせいになっちゃうもんね。
ねえ、雨の日ってさ、世界がちょっとだけ小さくなる感じ、しない？　聞こえるのは、雨と、わたしの声だけ。
こうやって並んで座ってると、屋根を叩く音が、ぜんぶ自分たちのためにあるみたいに聞こえるね。
【SCENE 02 · 耳かき】
耳、見せて。今日はこっちから、ゆっくりいくね。痛かったら、ちゃんと言うんだよ？
（耳かき、左。浅いところから、少しずつ奥へ。）
……痛くない？　よかった。けっこう溜まってるね。最近、忙しかった？
がんばってたんだね。えらいえらい。今日は、なんにも考えなくていいよ。
（梵天。ふわふわと、耳の入り口をなでる。）
はい、反対側。ごろんして。……そう、わたしの膝、使っていいから。
雨、まだ止みそうにないね。でも、止まなくてもいいかな。このまま、ずっと。
こっちの耳のほうが、ちょっとだけくすぐったがりだね。知ってた？　わたしは知ってたよ。
【SCENE 03 · 添い寝】
……眠くなってきた？　いいよ、寝ちゃっても。ちゃんと、ここにいるから。
（布団を広げる音。衣擦れ。）
手、つないでてあげる。……冷たいね。わたしの、あったかいでしょ。
おやすみ。明日は、晴れるといいね。晴れたら、どこか行こっか。
（雨音が、少しずつ弱まっていく。）
【SCENE 04 · 後日談】
おはよう。……ほら、晴れたよ。昨日の雨、ぜんぶどこかに行っちゃった。
約束、覚えてる？　どこか行こうって。……うん、覚えててくれて、うれしい。`

/** v3's longScript: repeat each scene's body so the gauge and the column measure something. */
function longScript(times: number): string {
  const out: string[] = []
  let body: string[] = []
  const flush = () => {
    for (let k = 0; k < times; k++) out.push(...body)
    body = []
  }
  for (const line of SCRIPT_SOURCE.split('\n')) {
    if (line.startsWith('【')) {
      flush()
      out.push(line)
    } else body.push(line)
  }
  flush()
  return out.join('\n')
}

const BODIES = new Map<string, string>([
  ['本編/lyrics/02_本編 ―雨音と耳かき―.lrc', LRC_02],
  ['本編/lyrics/04_おやすみ.lrc', LRC_04],
  ['台本/台本_全編.txt', longScript(4)],
  ['台本/台本_おまけ.txt', '【BONUS · 晴れた朝】\nおはよう。今日は、いい天気だね。\n洗濯物、いっしょに干そっか。'],
  ['特典/readme.txt', '特典音声は zip を展開してお聴きください。\n壁紙は PC 用とスマートフォン用の二種類です。'],
  ['はじめにお読みください.txt', 'このたびはお買い上げいただき、ありがとうございます。\nヘッドホン・イヤホンでの視聴を推奨します。\n本作品の無断転載・再配布を禁じます。'],
])

/* ═══ The tree ═══ */

const TREE: WorkContentDir = dir('', [
  dir('本編', [
    dir('本編/mp3', TRACKS.filter((_, t) => t < 7).map((track) => audio('MP3', track))),
    dir('本編/wav', TRACKS.map((track) => audio('WAV', track))),
    dir('本編/lyrics', [
      file('本編/lyrics/02_本編 ―雨音と耳かき―.lrc', 'lrc', { size: 6_348 }),
      file('本編/lyrics/04_おやすみ.lrc', 'lrc', { size: 1_843 }),
    ]),
  ]),
  dir('台本', [
    file('台本/台本_全編.txt', 'txt', { size: 90_112 }),
    file('台本/台本_おまけ.txt', 'txt', { size: 14_336 }),
  ]),
  dir('CG', [dir('CG/本編', cgMain), dir('CG/差分', cgDiff)]),
  dir('表紙', cover),
  dir('特典', [
    ...bonusImgs,
    file('特典/特典音声.zip', 'bin', { size: 222_298_112 }),
    file('特典/readme.txt', 'txt', { size: 2_048 }),
  ]),
  file('はじめにお読みください.txt', 'txt', { size: 3_072 }),
])

/** Any rj answers the same sample; the rj is echoed back (draft spec §3). */
export function loadWorkContent(rj: string): Promise<WorkContent> {
  return Promise.resolve({
    rj,
    copies: [
      { copy_id: 'copy-a', library_id: 'lib-a', dir_id: 'mock-dir-a', online: true, tree: TREE },
      { copy_id: 'copy-b', library_id: 'lib-b', dir_id: 'mock-dir-b', online: false, tree: dir('', []) },
    ],
  })
}

export function imageUrl(ref: ContentFileRef): string {
  return PIXELS.get(ref.relPath) ?? ''
}

/** Phase A = the full-size image (ADR 0015 §6 permits; the thumbnail route lands in B4). */
export function thumbUrl(ref: ContentFileRef, _heightPx: number): string {
  return imageUrl(ref)
}

/** The player stays silent until Phase D; nothing consumes this yet. */
export function audioUrl(_ref: ContentFileRef): string {
  return ''
}

export function loadText(ref: ContentFileRef): Promise<string> {
  const body = BODIES.get(ref.relPath)
  // A manifest file without a mock body answers empty; the real route always has bytes.
  return Promise.resolve(body ?? '')
}
