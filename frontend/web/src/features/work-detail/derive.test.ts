import { describe, expect, it } from 'vitest'
import {
  audioInfo,
  deriveTracks,
  dirCounts,
  dirAt,
  fileCount,
  fmtLong,
  fmtSize,
  fmtT,
  imageSets,
  isScrollShape,
  kindGlyph,
  lrcFilesOf,
  parseLrc,
  parseLyrics,
  parseTxt,
  playQueue,
  pickFormat,
  resumePoint,
  trackFormats,
} from './derive'
import type { WorkContentDir, WorkContentFile, WorkFileKind, WorkListening } from '@/lib/api/types'

/**
 * The pure derivations: everything the page reads off the manifest.

 * jsdom has no layout, so the geometry decisions live here as plain data:
 * which directory is a set, which set is the cover, what a track's slots are.
 */

function f(relPath: string, kind: WorkFileKind, extra: Partial<WorkContentFile> = {}): WorkContentFile {
  return {
    name: relPath.slice(relPath.lastIndexOf('/') + 1),
    rel_path: relPath,
    kind,
    size: 1024,
    format: null,
    bitrate: null,
    width: null,
    height: null,
    duration_sec: null,
    thumb_v: null,
    ...extra,
  }
}

function d(relPath: string, children: Array<WorkContentDir | WorkContentFile>): WorkContentDir {
  return { name: relPath === '' ? '' : relPath.slice(relPath.lastIndexOf('/') + 1), rel_path: relPath, children }
}

describe('imageSets', () => {
  it('makes every image-holding directory a set and puts the shallowest first', () => {
    // 表紙 and 特典 sit at depth 1, CG's two folders one deeper; the cover set
    // is the shallowest, and among equals the tree's own order wins.
    const tree = d('', [
      d('CG', [d('CG/本編', [f('CG/本編/a.png', 'img', { width: 100, height: 100 })])]),
      d('表紙', [f('表紙/main.jpg', 'img', { width: 560, height: 420 })]),
      d('特典', [f('特典/wp.png', 'img', { width: 100, height: 100 })]),
    ])
    const sets = imageSets(tree)
    expect(sets.map((set) => set.id)).toEqual(['表紙', '本編', '特典'])
    expect(sets[0].cover).toBe(true)
    expect(sets.filter((set) => set.cover)).toHaveLength(1)
  })

  it('keeps the root level working when covers sit flat in it', () => {
    const sets = imageSets(d('', [f('cover.jpg', 'img', { width: 560, height: 420 })]))
    expect(sets).toHaveLength(1)
    expect(sets[0].cover).toBe(true)
    expect(sets[0].path).toBe('')
  })

  it('answers no sets for a tree without images', () => {
    expect(imageSets(d('', [f('a.mp3', 'aud', { format: 'MP3' })]))).toEqual([])
  })
})

describe('deriveTracks', () => {
  const tree = d('', [
    d('本編/mp3', [
      f('本編/mp3/01_導入.mp3', 'aud', { format: 'MP3', duration_sec: 252 }),
      f('本編/mp3/02_本編.mp3', 'aud', { format: 'MP3', duration_sec: 2320 }),
    ]),
    d('本編/wav', [
      f('本編/wav/01_導入.wav', 'aud', { format: 'WAV', duration_sec: 252 }),
      f('本編/wav/02_本編.wav', 'aud', { format: 'WAV', duration_sec: 2320 }),
      f('本編/wav/03_後日談.wav', 'aud', { format: 'WAV', duration_sec: 480 }),
    ]),
    d('本編/lyrics', [f('本編/lyrics/02_本編.lrc', 'lrc')]),
  ])

  it('aggregates one track per shared basename with a slot per format', () => {
    const tracks = deriveTracks(tree)
    expect(tracks.map((track) => track.stem)).toEqual(['01_導入', '02_本編', '03_後日談'])
    expect(tracks[0].files).toHaveProperty('MP3')
    expect(tracks[0].files).toHaveProperty('WAV')
    expect(tracks[2].files.MP3).toBeUndefined()
    expect(tracks[2].formats).toEqual(['WAV'])
  })

  it('merges later formats without moving root files or prioritizing multi-format tracks', () => {
    const tree = d('', [
      d('mp3', [10, 2, 1].map((n) => f(`mp3/${n}.mp3`, 'aud', { format: 'MP3' }))),
      f('10.wav', 'aud', { format: 'WAV' }),
      d('bonus', [d('bonus/ex', [f('bonus/ex/bonus.mp3', 'aud', { format: 'MP3' })])]),
      f('2.wav', 'aud', { format: 'WAV' }),
      f('1.wav', 'aud', { format: 'WAV' }),
    ])
    const original = JSON.stringify(tree)
    const tracks = deriveTracks(tree)
    expect(tracks.map((track) => track.stem)).toEqual(['1', '2', '10', 'bonus'])
    expect(tracks[0].files.WAV?.rel_path).toBe('1.wav')
    expect(tracks[0].files.MP3?.rel_path).toBe('mp3/1.mp3')
    expect(trackFormats(tracks)).toEqual(['MP3', 'WAV'])
    expect(JSON.stringify(tree)).toBe(original)

    tree.children.push(f('0.wav', 'aud', { format: 'WAV' }))
    expect(deriveTracks(tree).map((track) => track.stem)).toEqual(['0', '1', '2', '10', 'bonus'])
  })

  it('finishes each naturally ordered directory before its next sibling', () => {
    const tree = d('', [
      d('10', [f('10/a.mp3', 'aud', { format: 'MP3' })]),
      d('2', [
        d('2/child', [f('2/child/b.mp3', 'aud', { format: 'MP3' })]),
        f('2/z.mp3', 'aud', { format: 'MP3' }),
      ]),
      f('root.mp3', 'aud', { format: 'MP3' }),
    ])
    expect(deriveTracks(tree).map((track) => track.stem)).toEqual(['root', 'z', 'b', 'a'])
  })

  it('puts MP3 and WAV chips before other formats, regardless of discovery order', () => {
    const tracks = deriveTracks(d('', ['FLAC', 'WAV', 'OGG', 'MP3'].map((format) =>
      f(`track.${format.toLowerCase()}`, 'aud', { format }),
    )))
    expect(trackFormats(tracks)).toEqual(['MP3', 'WAV', 'FLAC', 'OGG'])
  })

  it('pairs the same-basename .lrc automatically', () => {
    const tracks = deriveTracks(tree)
    expect(tracks[1].lrcRel).toBe('本編/lyrics/02_本編.lrc')
    expect(tracks[0].lrcRel).toBeNull()
  })

  it('pairs WebVTT lyrics using the same manifest kind', () => {
    const tracks = deriveTracks(d('', [f('a.mp3', 'aud', { format: 'MP3' }), f('a.vtt', 'lrc')]))
    expect(tracks[0].lrcRel).toBe('a.vtt')
  })

  it.each([
    [['a.lrc', 'a.VTT'], 'a.VTT'],
    [['a.VTT', 'a.lrc'], 'a.VTT'],
    [['a.lrc'], 'a.lrc'],
    [['other.vtt', 'a.lrc'], 'a.lrc'],
    [['other.vtt'], null],
    [['a.mp3.vtt'], 'a.mp3.vtt'],
    [['a.mp3.lrc'], 'a.mp3.lrc'],
    [['a.lrc', 'a.mp3.VTT'], 'a.mp3.VTT'],
    [['a.VTT', 'a.mp3.lrc'], 'a.VTT'],
    [['a.vtt', 'a.mp3.vtt', 'a.lrc'], 'a.mp3.vtt'],
    [['a.wav.vtt', 'a.mp3.vtt.bak'], null],
  ] as const)('prefers VTT with LRC fallback for matching lyric names %j', (names, expected) => {
    const tracks = deriveTracks(d('', [
      f('a.mp3', 'aud', { format: 'MP3' }),
      ...names.map((name) => f(`lyrics/${name}`, 'lrc')),
    ]))
    expect(tracks[0].lrcRel).toBe(expected === null ? null : `lyrics/${expected}`)
  })

  const aud = (path: string) => {
    const ext = path.slice(path.lastIndexOf('.') + 1)
    return f(path, 'aud', { format: ext.toUpperCase() })
  }
  const layout = (...paths: string[]) => d('', paths.map(aud))

  it.each(['vtt', 'lrc'])('pairs full audio filenames from any format of a track (%s)', (extension) => {
    const tree = d('', [
      ...['mp3/預約電話.mp3', 'wav/預約電話.wav'].map(aud),
      f(`預約電話.wav.${extension}`, 'lrc'),
    ])
    const tracks = deriveTracks(tree)
    expect(tracks).toHaveLength(1)
    expect(tracks[0].lrcRel).toBe(`預約電話.wav.${extension}`)
  })

  it.each([
    ['plain format folders', ['wav /1.wav', 'wav /2.wav', 'mp3/1.mp3', 'mp3/2.mp3'], ['1', '2'], 2],
    ['format-prefixed variant folders',
      ['wav_se_on/1.wav', 'wav_se_off/1.wav', 'mp3_se_on/1.mp3', 'mp3_se_off/1.mp3'], ['se_off/1', 'se_on/1'], 2],
    ['variant under format folder', ['wav/SE有/1.wav', 'wav/SE無/1.wav', 'mp3/SE有/1.mp3', 'mp3/SE無/1.mp3'], ['SE有/1', 'SE無/1'], 2],
    ['format under variant folder', ['SE有/wav/1.wav', 'SE有/mp3/1.mp3'], ['SE有/1'], 2],
    ['decorated format folders', ['WAV版/1.wav', 'mp3 (320kbps)/1.mp3'], ['1'], 2],
  ])('keeps variants apart and merges formats: %s', (_name, paths, keys, formatCount) => {
    const tracks = deriveTracks(layout(...paths))
    expect(tracks.map((track) => track.key).sort()).toEqual(keys)
    for (const track of tracks) expect(track.formats).toHaveLength(formatCount)
  })

  it('keeps both files when a format slot would be overwritten', () => {
    const tracks = deriveTracks(layout('mp3_320/01.mp3', 'mp3_128/01.mp3'))
    expect(tracks).toHaveLength(2)
    expect(tracks.map((track) => track.files.MP3?.rel_path).sort()).toEqual(['mp3_128/01.mp3', 'mp3_320/01.mp3'])
  })

  it('pairs lyrics from the same variant first', () => {
    const tree = d('', [
      ...['SE有/wav/1.wav', 'SE無/wav/1.wav'].map(aud),
      f('SE有/1.lrc', 'lrc'),
      f('SE無/1.lrc', 'lrc'),
    ])
    expect(deriveTracks(tree).map((track) => track.lrcRel)).toEqual(['SE有/1.lrc', 'SE無/1.lrc'])
  })

  it('prefers same-variant LRC over other-variant VTT across both naming styles', () => {
    const tree = d('', [
      ...['SE有/mp3/1.mp3', 'SE無/mp3/1.mp3'].map(aud),
      f('SE有/1.mp3.lrc', 'lrc'),
      f('SE無/1.vtt', 'lrc'),
    ])
    expect(deriveTracks(tree).map((track) => track.lrcRel)).toEqual(['SE有/1.mp3.lrc', 'SE無/1.vtt'])
  })

  it('keeps the duration of whichever format reported one', () => {
    expect(deriveTracks(tree)[0].duration).toBe(252)
  })
})

describe('playQueue', () => {
  const tracks = deriveTracks(d('', [
    'wav_se_on/1.wav', 'wav_se_on/2.wav', 'wav_se_off/1.wav', 'wav_se_off/2.wav',
    'mp3_se_on/1.mp3', 'mp3_se_off/1.mp3',
  ].map((p) => f(p, 'aud', { format: p.slice(p.lastIndexOf('.') + 1).toUpperCase() }))))
  const keys = (queue: { key?: string }[]) => queue.map((item) => item.key)

  it('queues one codec of one variant only', () => {
    const { queue, index, format } = playQueue(tracks, 0, 'WAV')
    expect(format).toBe('WAV')
    expect(keys(queue)).toEqual(['se_on/1', 'se_on/2'])
    expect(index).toBe(0)
    expect(keys(playQueue(tracks, 0, 'MP3').queue)).toEqual(['se_on/1'])
  })

  it('gives each entry its Timed Lyrics: the manual pairing first, then the automatic one', () => {
    const withLrc = deriveTracks(d('', [
      f('a.mp3', 'aud', { format: 'MP3' }), f('b.mp3', 'aud', { format: 'MP3' }), f('c.mp3', 'aud', { format: 'MP3' }),
      f('a.lrc', 'lrc'), f('b.lrc', 'lrc'), f('manual.lrc', 'lrc'),
    ]))
    const { queue } = playQueue(withLrc, 0, 'MP3', undefined, { copyId: 'copy-a', pairs: { 1: 'manual.lrc' } })
    expect(queue.map((item) => item.lyrics)).toEqual([
      { copyId: 'copy-a', relPath: 'a.lrc' },
      { copyId: 'copy-a', relPath: 'manual.lrc' },
      undefined,
    ])
    expect(playQueue(withLrc, 0, 'MP3').queue[0].lyrics).toBeUndefined()
  })

  it('maps the start track to its place in the filtered queue', () => {
    const second = tracks.findIndex((t) => t.key === 'se_off/2')
    const { queue, index } = playQueue(tracks, second, 'WAV')
    expect(keys(queue)).toEqual(['se_off/1', 'se_off/2'])
    expect(index).toBe(1)
  })

  it('plays a track lacking the codec in its own first format', () => {
    const second = tracks.findIndex((t) => t.key === 'se_on/2')
    const { queue, index, format } = playQueue(tracks, second, 'MP3')
    expect(format).toBe('WAV')
    expect(keys(queue)).toEqual(['se_on/1', 'se_on/2'])
    expect(index).toBe(1)
  })

  it('carries each entry\'s Rendition and its place among all of that Rendition\'s tracks', () => {
    // se_on has two tracks in WAV, one in MP3: the MP3 queue is cut short, the count is not.
    const { queue } = playQueue(tracks, 0, 'MP3')
    expect(queue.map(({ rendition, index, count }) => ({ rendition, index, count })))
      .toEqual([{ rendition: 'se_on', index: 0, count: 2 }])
    const wav = playQueue(tracks, tracks.findIndex((t) => t.key === 'se_off/2'), 'WAV').queue
    expect(wav.map(({ rendition, index, count }) => [rendition, index, count]))
      .toEqual([['se_off', 0, 2], ['se_off', 1, 2]])
  })

  it('answers an empty queue for a missing start track', () => {
    expect(playQueue(tracks, 99, 'WAV').queue).toEqual([])
  })
})

describe('lrcFilesOf', () => {
  it('uses files-first natural depth-first order for all lyric formats without merging files', () => {
    const tree = d('', [
      d('10', [f('10/a.lrc', 'lrc')]),
      d('2', [
        d('2/nested', [f('2/nested/1.lrc', 'lrc')]),
        f('2/z.vtt', 'lrc'),
      ]),
      f('10.lrc', 'lrc'),
      f('2.vtt', 'lrc'),
      f('1.lrc', 'lrc'),
      f('audio.mp3', 'aud', { format: 'MP3' }),
    ])
    const original = JSON.stringify(tree)
    expect(lrcFilesOf(tree).map((file) => file.rel_path)).toEqual([
      '1.lrc', '2.vtt', '10.lrc', '2/z.vtt', '2/nested/1.lrc', '10/a.lrc',
    ])
    expect(JSON.stringify(tree)).toBe(original)
  })
})

describe('tree walking', () => {
  const tree = d('', [
    d('a', [f('a/x.png', 'img'), f('a/y.txt', 'txt'), d('a/b', [f('a/b/z.wav', 'aud', { format: 'WAV' })])]),
    f('root.lrc', 'lrc'),
    f('clip.mp4', 'vid'),
  ])

  it('counts recursively, lrc as txt', () => {
    expect(dirCounts(tree)).toEqual({ aud: 1, img: 1, txt: 2, vid: 1 })
    expect(fileCount(tree)).toBe(5)
  })

  it('resolves a breadcrumb to its directory', () => {
    expect(dirAt(tree, ['a', 'b'])?.name).toBe('b')
    expect(dirAt(tree, ['gone'])).toBeNull()
  })
})

describe('parseLrc', () => {
  it('keeps every timestamped line in Windows CRLF files, including a final line without a newline', () => {
    const lines = parseLrc('\uFEFF[ti:track]\r\n[00:03.50]first\r\n[00:10.83]second\r\n[04:37.23]last')
    expect(lines).toEqual([
      { time: 3.5, text: 'first', sfx: false },
      { time: 10.83, text: 'second', sfx: false },
      { time: 277.23, text: 'last', sfx: false },
    ])
  })

  it('takes the first tag of a line and skips untagged ones', () => {
    const lines = parseLrc('[00:09.00]あ、降ってきたね。\n[00:15][00:20.5]こっち。\nplain line')
    expect(lines).toEqual([
      { time: 9, text: 'あ、降ってきたね。', sfx: false },
      { time: 15, text: 'こっち。', sfx: false },
    ])
  })

  it('drops inline word-timing tags from the line text', () => {
    expect(parseLrc('[00:01.00]<00:01.00>あ<00:01.50>い<00:02.00>う')).toEqual([{ time: 1, text: 'あいう', sfx: false }])
  })

  it('marks full-line parentheticals as sound effects', () => {
    expect(parseLrc('[01:00.00]（耳かきの音、左から）')[0].sfx).toBe(true)
    expect(parseLrc('[01:11.00]痛くない？')[0].sfx).toBe(false)
  })
})

describe('parseLyrics', () => {
  it('reads WebVTT identifiers, settings, hours, multiline text, tags and entities', () => {
    const text = '\uFEFFWEBVTT\r\n\r\nNOTE comment\r\n00:00.000 --> 00:01.000\r\nignored\r\n\r\nSTYLE\r\n::cue { color: red; }\r\n\r\ncue-1\r\n01:02:03.125 --> 01:02:05.000 align:start\r\n<v speaker><b>hello</b> &amp; &lt;world&gt;\r\nsecond line\r\n\r\n00:01.000 --> 00:02.000\r\n（音）'
    expect(parseLyrics(text, 'a.VTT')).toEqual([
      { time: 1, endTime: 2, text: '（音）', sfx: true },
      { time: 3723.125, endTime: 3725, text: 'hello & <world>\nsecond line', sfx: false },
    ])
  })

  it('rejects invalid WebVTT headers and timestamps, and keeps LRC supported', () => {
    expect(parseLyrics('not WEBVTT\n\n00:01.000 --> 00:02.000\ntext', 'a.vtt')).toEqual([])
    expect(parseLyrics('WEBVTT\n\n00:99.000 --> 01:00.000\nbad\n\n00:02.000 --> 00:01.000\nbackwards', 'a.vtt')).toEqual([])
    expect(parseLyrics('[00:10]later\n[00:01]first', 'a.lrc').map((line) => line.time)).toEqual([1, 10])
  })
})

describe('parseTxt', () => {
  it('reads the typography as the only structure', () => {
    const lines = parseTxt('【SCENE 01 · 縁側】\n（雨音。遠雷。）\nあ、降ってきたね。')
    expect(lines).toEqual([
      { kind: 'scene', text: 'SCENE 01 · 縁側' },
      { kind: 'sfx', text: '雨音。遠雷。' },
      { kind: 'sp', text: 'あ、降ってきたね。' },
    ])
  })
})

describe('telemetry formatting', () => {
  it('formats durations, long durations, sizes and shapes', () => {
    expect(fmtT(252)).toBe('04:12')
    expect(fmtT(3720)).toBe('62:00')
    expect(fmtLong(3720)).toBe('1H02M')
    expect(fmtLong(252)).toBe('04:12')
    expect(fmtSize(6_348)).toBe('6.2K')
    expect(fmtSize(222_298_112)).toBe('212.0M')
    expect(kindGlyph('dir')).toBe('▸')
    expect(kindGlyph('lrc')).toBe('¶')
    expect(kindGlyph('vid')).toBe('▶')
    expect(isScrollShape(800, 2400)).toBe(true)
    expect(isScrollShape(1200, 1800)).toBe(false)
    expect(isScrollShape(null, 100)).toBe(false)
  })

  it('labels an audio file by its duration and format', () => {
    expect(audioInfo(f('a.mp3', 'aud', { format: 'MP3', bitrate: 320, duration_sec: 252 }))).toBe('04:12 · MP3 320K')
    expect(audioInfo(f('a.wav', 'aud', { format: 'WAV', duration_sec: 480 }))).toBe('08:00 · WAV')
  })
})

describe('pickFormat', () => {
  it('takes the first preferred format the work ships, else null', () => {
    expect(pickFormat(['MP3', 'WAV'], ['FLAC', 'WAV', 'MP3'])).toBe('WAV')
    expect(pickFormat(['MP3', 'WAV'], ['FLAC'])).toBeNull()
    expect(pickFormat(['MP3', 'WAV'], [])).toBeNull()
  })
})

describe('resumePoint', () => {
  const tracks = deriveTracks(d('', ['1.wav', '2.wav'].map((p) => f(p, 'aud', { format: 'WAV' }))))
  const row = (extra: Partial<WorkListening>): WorkListening => ({
    rendition: '', track_key: '2', position_sec: 30, track_index: 1, track_count: 2,
    finished_at: null, updated_at: '2026-10-01T00:00:00.000000000Z', ...extra,
  })

  it('picks the most recently updated row with a position', () => {
    const older = row({ track_key: '1', position_sec: 5, updated_at: '2026-09-01T00:00:00.000000000Z' })
    expect(resumePoint([older, row({})], tracks)).toMatchObject({ track: 1, position: 30 })
  })

  it('skips a Finished row and an orphan row for the next one', () => {
    const finished = row({ track_key: null, position_sec: null, track_index: null, track_count: null, finished_at: '2026-10-02T00:00:00.000000000Z', updated_at: '2026-10-02T00:00:00.000000000Z' })
    const orphan = row({ rendition: 'gone', track_key: 'gone/9', updated_at: '2026-10-03T00:00:00.000000000Z' })
    const older = row({ track_key: '1', position_sec: 5, updated_at: '2026-09-01T00:00:00.000000000Z' })
    expect(resumePoint([finished, orphan, older], tracks)).toMatchObject({ track: 0, position: 5 })
  })

  it('answers null when nothing is left to resume', () => {
    expect(resumePoint([row({ track_key: 'missing' })], tracks)).toBeNull()
    expect(resumePoint([], tracks)).toBeNull()
  })
})
