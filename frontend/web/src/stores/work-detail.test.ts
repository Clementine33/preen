import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { sampleWorkContent } from '@/test/work-content-fixture'
import { findFile } from '@/test/work-fixture'
import { usePlayerStore } from './player'
import { useWorkDetailStore } from './work-detail'

/**
 * The session state of the detail page: one work is one
 * session, followTrack never interrupts a script being read, a track missing
 * the browsed format plays in its only one, and every selection clamps or
 * resets against the bound manifest.
 */

const RJ = 'RJ01000001'
const OTHER = 'RJ02000002'

async function bind(rj = RJ): Promise<void> {
  const store = useWorkDetailStore()
  store.bind(rj, sampleWorkContent(rj))
}

/** A real file object from the bound manifest, addressed by rel_path. */
function fileAt(relPath: string) {
  return findFile(useWorkDetailStore().copy!, relPath)
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('work-detail store', () => {
  it('binds the manifest and picks the first format for the chips', async () => {
    await bind()
    const store = useWorkDetailStore()
    expect(store.rj).toBe(RJ)
    expect(store.formats).toEqual(['MP3', 'WAV'])
    expect(store.fmt).toBe('MP3')
    expect(store.sets.length).toBeGreaterThan(1)
    expect(store.sets[0].cover).toBe(true)
    expect(store.tracks).toHaveLength(8)
  })

  it('opens on the preferred format, ignoring preferences the work does not ship', () => {
    const store = useWorkDetailStore()
    store.bind(RJ, sampleWorkContent(RJ), undefined, ['FLAC', 'WAV', 'MP3'])
    expect(store.fmt).toBe('WAV')
    expect(store.formats).toEqual(['MP3', 'WAV'])
    store.bind(OTHER, sampleWorkContent(OTHER), undefined, ['FLAC'])
    expect(store.fmt).toBe('MP3')
  })

  it('resets the session when another work binds, and keeps it on a refetch', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.scriptSeen = true
    store.pairs[2] = '本編/lyrics/02_本編 ―雨音と耳かき―.lrc'
    store.unveiled = true
    store.setDock('right')
    store.openBayTab('files')

    store.bind(OTHER, sampleWorkContent(OTHER))
    expect(store.rj).toBe(OTHER)
    expect(store.scriptSeen).toBe(false)
    expect(store.pairs).toEqual({})
    expect(store.unveiled).toBe(false)
    expect(store.dock).toBe('right')
    expect(store.tab).toBe('files')

    store.scriptSeen = true
    store.bind(OTHER, sampleWorkContent(OTHER))
    expect(store.scriptSeen).toBe(true)
  })

  it('clamps stepping and stepping always shows the front', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.flip()
    expect(store.flipped).toBe(true)
    store.step(-5)
    expect(store.flipped).toBe(false)
    expect(store.idx).toBe(0)
    store.step(999)
    expect(store.idx).toBe(store.currentSet!.files.length - 1)
  })

  it('switching sets resets the index and the flip', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.flip()
    store.showSet(1)
    expect(store.flipped).toBe(false)
    expect(store.idx).toBe(0)
    expect(store.set).toBe(1)
  })

  it('keeps READ sticky once opened and refuses it without a text', async () => {
    await bind()
    const store = useWorkDetailStore()
    expect(store.scriptSeen).toBe(false)
    store.setLeaf('script')
    expect(store.leaf).toBe('colophon')
    store.selectFile(fileAt('台本/台本_全編.txt'))
    expect(store.leaf).toBe('script')
    expect(store.scriptSeen).toBe(true)
    expect(store.activeTxt).toBe('台本/台本_全編.txt')
  })

  it('follows a track without interrupting a script being read', async () => {
    await bind()
    const store = useWorkDetailStore()
    // Track 2 (index 1) has the automatic pairing; track 1 has none.
    store.startTrack(1)
    expect(store.leaf).toBe('lrc')
    store.startTrack(0)
    expect(store.leaf).toBe('colophon')

    store.activeTxt = '台本/台本_全編.txt'
    store.leaf = 'script'
    store.startTrack(1)
    expect(store.leaf).toBe('script')
  })

  it('plays a track missing the browsed format in its only format', async () => {
    await bind()
    const store = useWorkDetailStore()
    const player = usePlayerStore()
    store.startTrack(7) // 08 exists only as WAV
    expect(store.playFmt).toBe('WAV')
    expect(player.rj).toBe(RJ)
    expect(player.trackIndex).toBe(7)
    expect(player.title).toBe(store.tracks[7].stem)
    store.startTrack(0, 'WAV')
    expect(store.playFmt).toBe('WAV')
  })

  it('starts a chosen track at 0 s and a resume at its saved second', async () => {
    await bind()
    const store = useWorkDetailStore()
    const player = usePlayerStore()
    store.startTrack(2, undefined, 271)
    expect([player.trackIndex, player.elapsed]).toEqual([2, 271])
    store.startTrack(2)
    expect([player.trackIndex, player.elapsed]).toEqual([2, 0])
  })

  it('automatically opens same-stem VTT on playback, with LRC fallback and manual overrides', () => {
    const store = useWorkDetailStore()
    const content = structuredClone(sampleWorkContent(RJ))
    const tree = content.copies[0].tree!
    const lrc = findFile(tree, '本編/lyrics/02_本編 ―雨音と耳かき―.lrc')
    const name = lrc.name.replace(/\.lrc$/, '.vtt')
    tree.children.push({ ...lrc, name, rel_path: name })
    store.bind(RJ, content)
    store.startTrack(1)
    expect(store.lrcFor(1)).toBe(name)
    expect(store.leaf).toBe('lrc')
    store.startTrack(3)
    expect(store.lrcFor(3)).toBe('本編/lyrics/04_おやすみ.lrc')
    store.pairLrc(1, lrc.rel_path)
    store.startTrack(1)
    expect(store.lrcFor(1)).toBe(lrc.rel_path)
  })

  it('plays the chosen format file by its address, with the scanned length', () => {
    const store = useWorkDetailStore()
    store.bind(RJ, sampleWorkContent(RJ), (ref) => `/files/${ref.copyId}/${ref.relPath}`)
    const player = usePlayerStore()
    const playing = () => player.queue[player.trackIndex].source?.src

    store.startTrack(7) // WAV only: the fallback format's file plays
    expect(playing()).toBe('/files/copy-a/本編/wav/08_後日談.wav')
    expect(player.format).toBe('WAV')
    expect(player.duration).toBe(480)

    store.startTrack(0)
    expect(playing()).toBe('/files/copy-a/本編/mp3/01_導入 ―縁側にて―.mp3')
    expect(player.format).toBe('MP3')
    // Only the tracks that exist in MP3 queue, so next never leaves MP3.
    expect(player.queue).toHaveLength(store.tracks.filter((track) => track.files.MP3).length)
    expect(player.queue.every((item) => item.source?.format === 'MP3')).toBe(true)
    expect(player.trackKey).toBe(store.tracks[0].key)
  })

  it('pairs lyrics by hand through the LRC chip and follows into LISTEN', async () => {
    await bind()
    const store = useWorkDetailStore()
    expect(store.lrcFor(0)).toBeNull()
    store.pairLrc(0, '本編/lyrics/04_おやすみ.lrc')
    expect(store.lrcFor(0)).toBe('本編/lyrics/04_おやすみ.lrc')
    expect(store.leaf).toBe('lrc')
  })

  it('acts on file rows: an image locates its set, text and lyrics open READ', async () => {
    await bind()
    const store = useWorkDetailStore()
    const img = store.sets[1].files[0]
    store.selectFile(img)
    expect(store.set).toBe(1)
    expect(store.idx).toBe(0)

    const audio = Object.values(store.tracks[1].files)[0]!
    store.selectFile(audio)
    expect(usePlayerStore().trackIndex).toBe(1)

    const lyric = fileAt('本編/lyrics/04_おやすみ.lrc')
    store.selectFile(lyric)
    expect(store.leaf).toBe('script')
    expect(store.activeTxt).toBe(lyric.rel_path)
    expect(store.pairs).toEqual({})
    expect(usePlayerStore().trackIndex).toBe(1)

    store.selectFile(fileAt('台本/台本_全編.txt'))
    expect(store.leaf).toBe('script')
    // A non-audio pick closes the bottom drawer (v3).
    expect(store.bayOpen).toBe(false)
  })

  it('keeps the bottom drawer when the pick is audio', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.openBayTab('files')
    expect(store.bayOpen).toBe(true)
    const audio = Object.values(store.tracks[0].files)[0]!
    store.selectFile(audio)
    expect(store.bayOpen).toBe(true)
  })

  it('reads selected lyrics without replacing the current track’s manual pairing', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.pairLrc(1, '本編/lyrics/04_おやすみ.lrc')
    const lyric = fileAt('本編/lyrics/02_本編 ―雨音と耳かき―.lrc')
    store.selectFile(lyric)
    expect(store.lrcFor(1)).toBe('本編/lyrics/04_おやすみ.lrc')
    expect(store.activeTxt).toBe(lyric.rel_path)
    expect(store.leaf).toBe('script')
    expect(store.track).toBe(0)
  })

  it('lifts a tab click and closes the open tab clicked again', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.openBayTab('tracks')
    expect(store.bayOpen).toBe(true)
    store.openBayTab('tracks')
    expect(store.bayOpen).toBe(false)
    store.openBayTab('files')
    expect(store.bayOpen).toBe(true)
    expect(store.tab).toBe('files')
  })
})
