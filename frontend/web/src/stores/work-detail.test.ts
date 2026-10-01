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

  it('resets the session when another work binds, and keeps it on a refetch', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.scriptSeen = true
    store.pairs[2] = '本編/lyrics/02_本編 ―雨音と耳かき―.lrc'
    store.veil = true

    store.bind(OTHER, sampleWorkContent(OTHER))
    expect(store.rj).toBe(OTHER)
    expect(store.scriptSeen).toBe(false)
    expect(store.pairs).toEqual({})
    expect(store.veil).toBe(false)

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

  it('plays the chosen format file by its address, with the scanned length', () => {
    const store = useWorkDetailStore()
    const addresses: string[] = []
    store.bind(RJ, sampleWorkContent(RJ), (ref) => {
      addresses.push(`${ref.copyId}:${ref.relPath}`)
      return `/files/${ref.relPath}`
    })
    const player = usePlayerStore()

    store.startTrack(7) // WAV only: the fallback format's file plays
    expect(addresses.at(-1)).toBe('copy-a:本編/wav/08_後日談.wav')
    expect(player.format).toBe('WAV')
    expect(player.duration).toBe(480)

    store.startTrack(0)
    expect(addresses.at(-1)).toBe('copy-a:本編/mp3/01_導入 ―縁側にて―.mp3')
    expect(player.format).toBe('MP3')
  })

  it('pairs lyrics by hand through the menu action and follows into LISTEN', async () => {
    await bind()
    const store = useWorkDetailStore()
    expect(store.lrcFor(0)).toBeNull()
    store.pairLrc(0, '本編/lyrics/04_おやすみ.lrc')
    expect(store.lrcFor(0)).toBe('本編/lyrics/04_おやすみ.lrc')
    expect(store.leaf).toBe('lrc')
  })

  it('acts on file rows: an image locates its set, a text opens READ, a lyric pairs and plays', async () => {
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
    expect(store.leaf).toBe('lrc')
    // Its automatic pairing exists, so no manual pair is recorded.
    expect(store.pairs).toEqual({})
    expect(usePlayerStore().trackIndex).toBe(3)

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

  it('resets the veil visit when the veil toggles', async () => {
    await bind()
    const store = useWorkDetailStore()
    store.veil = true
    store.unveiled = true
    store.toggleVeil()
    expect(store.veil).toBe(false)
    store.toggleVeil()
    expect(store.unveiled).toBe(false)
  })
})
