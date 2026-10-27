import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClientKey } from '@/lib/api/client'
import { usePlayerStore } from '@/stores/player'
import { useWorkDetailStore } from '@/stores/work-detail'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { sampleWorkContent } from '@/test/work-content-fixture'
import { makeWorkDetail } from '@/test/work-fixture'
import WorkBay from './WorkBay.vue'

/**
 * The bay's contracts: a tab click lifts, the open tab's
 * click closes; format chips keep the merged track list without changing
 * playback indices or lyric pairings;
 * the FILES tab's rows act on the whole page.
 */

const RJ = 'RJ01000001'

async function mountBay() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useWorkDetailStore()
  store.bind(RJ, sampleWorkContent(RJ))
  const api = apiStub({
    getWork: vi.fn().mockResolvedValue(makeWorkDetail()),
  } as never)
  const wrapper = mount(WorkBay, {
    global: {
      plugins: [
        pinia,
        installTestQueryPlugin(),
      ],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  await flushPromises()
  return { wrapper, store }
}

beforeEach(() => {
  vi.clearAllMocks()
})

enableAutoUnmount(afterEach)

describe('workBay', () => {
  it('counts the tabs and lifts on the first tab click, closing on the second', async () => {
    const { wrapper, store } = await mountBay()
    const tabs = wrapper.findAll('.tab')
    expect(tabs[0].text()).toContain('TRACKS')
    expect(tabs[0].text()).toContain('8')
    expect(tabs[1].text()).toContain('FILES')
    expect(store.bayOpen).toBe(false)
    await tabs[0].trigger('click')
    expect(store.bayOpen).toBe(true)
    await tabs[0].trigger('click')
    expect(store.bayOpen).toBe(false)
  })

  it('lists mixed formats and plays a missing-format track in its available format', async () => {
    const { wrapper } = await mountBay()
    const store = useWorkDetailStore()
    const player = usePlayerStore()
    store.openBayTab('tracks')
    await flushPromises()
    const rows = wrapper.findAll('.trk')
    expect(rows).toHaveLength(8)
    const last = rows[7]
    expect(last.classes()).toContain('missing')
    expect(last.text()).toContain('WAV ONLY')
    expect(last.text()).not.toContain('¶') // 08 has no paired lyric
    await last.trigger('click')
    expect(player.trackIndex).toBe(7)
    expect(store.playFmt).toBe('WAV')
  })

  it('switches audio chips without changing order, playback or manual lyric pairing', async () => {
    const { wrapper, store } = await mountBay()
    const player = usePlayerStore()
    store.pairLrc(1, store.lrcFiles[1].rel_path)
    store.startTrack(1)
    const paired = store.lrcFor(1)
    store.openBayTab('tracks')
    await flushPromises()
    const chips = wrapper.findAll('.chip')
    const before = wrapper.findAll('.trk').map(row => [
      row.get('.no').text(),
      row.get('.nm').text(),
    ])
    expect(chips.map(chip => chip.attributes('aria-pressed'))).toEqual(['true', 'false', 'false'])
    await chips[1].trigger('click')
    expect(store.fmt).toBe('WAV')
    const rows = wrapper.findAll('.trk')
    expect(rows).toHaveLength(8)
    expect(rows.map(row => [
      row.get('.no').text(),
      row.get('.nm').text(),
    ])).toEqual(before)
    expect(rows[7].classes()).not.toContain('missing')
    expect(rows[7].text()).not.toContain('ONLY')
    expect(rows[7].find('.only').exists()).toBe(false)
    expect(player.trackIndex).toBe(1)
    expect(store.playFmt).toBe('MP3')
    expect(store.lrcFor(1)).toBe(paired)
    expect(rows[1].attributes('aria-current')).toBe('true')
    await chips[0].trigger('click')
    expect(wrapper.findAll('.trk').map(row => [
      row.get('.no').text(),
      row.get('.nm').text(),
    ])).toEqual(before)
    expect(wrapper.findAll('.trk')[7].text()).toContain('WAV ONLY')
    expect(store.lrcFor(1)).toBe(paired)
  })

  it('keeps merged indices and uses selected or fallback file durations', async () => {
    const { wrapper, store } = await mountBay()
    const wav = store.tracks[0].files.WAV!
    const mp3 = store.tracks[0].files.MP3!
    const lyric = store.lrcFiles[0]
    const content = sampleWorkContent(RJ)
    store.bind(RJ, {
      ...content,
      copies: [
        {
          ...content.copies[0],
          tree: {
            ...store.copy!,
            children: [
              { ...wav, name: '0.wav', rel_path: '0.wav' },
              {
                ...wav,
                name: '1.wav',
                rel_path: '1.wav',
                duration_sec: 120,
              },
              {
                ...wav,
                name: '2.wav',
                rel_path: '2.wav',
                duration_sec: 240,
              },
              {
                name: 'mp3',
                rel_path: 'mp3',
                children: [
                  {
                    ...mp3,
                    name: '1.mp3',
                    rel_path: 'mp3/1.mp3',
                    duration_sec: 30,
                  },
                  {
                    ...mp3,
                    name: '2.mp3',
                    rel_path: 'mp3/2.mp3',
                    duration_sec: null,
                  },
                ],
              },
              { ...lyric, name: '1.vtt', rel_path: '1.vtt' },
            ],
          },
        },
      ],
    }, ref => `/media/${ref.relPath}`)
    await flushPromises()
    const rows = wrapper.findAll('.trk')
    expect(rows.map(row => row.get('.no').text())).toEqual(['01', '02', '03'])
    expect(rows.map(row => row.get('.nm').text())).toEqual(['0', '1', '2'])
    expect(rows.map(row => row.get('.dur').text())).toEqual(['04:12', '00:30', '—'])
    expect(rows.map(row => row.get('.bar').attributes('style'))).toEqual(['width: 160px;', 'width: 19px;', 'width: 0px;'])
    expect(wrapper.get('.bay-sum').text()).toContain('00:30')
    await rows[1].trigger('click')
    // Track 0 is WAV only, so the MP3 queue is [1, 2]; the key still names row 1.
    expect(usePlayerStore().trackKey).toBe(store.tracks[1].key)
    expect(usePlayerStore().format).toBe('MP3')
    expect(store.lrcFor(1)).toBe('1.vtt')
    expect(store.leaf).toBe('lrc')
    expect(wrapper.findAll('.trk')[1].attributes('aria-current')).toBe('true')
    // The lyric chip still targets the same merged index.
    await wrapper.findAll('.chip')[2].trigger('click')
    expect(wrapper.findAll('.trk')[0].get('.dur').text()).toBe('TRACK 02')
    await wrapper.findAll('.trk')[0].trigger('click')
    expect(store.pairs[1]).toBe('1.vtt')
    await wrapper.findAll('.chip')[1].trigger('click')
    expect(wrapper.findAll('.trk').map(row => row.get('.nm').text())).toEqual(['0', '1', '2'])
    expect(wrapper.findAll('.trk')[1].get('.dur').text()).toBe('02:00')
    expect(store.playFmt).toBe('MP3')
  })

  it('navigates directories and acts on file rows', async () => {
    const { wrapper, store } = await mountBay()
    const player = usePlayerStore()
    store.openBayTab('files')
    await flushPromises()

    // The browser opens at the copy root; the first directory is 本編.
    const dirRow = wrapper.findAll('.row.dir').find(row => row.text().includes('本編'))!
    await dirRow.trigger('click')
    expect(store.cwd).toEqual(['本編'])
    // The up-row returns.
    await wrapper.get('.row.dir').trigger('click')
    expect(store.cwd).toEqual([])

    // An image row locates its set on the deck: enter 表紙 first.
    const coverDir = wrapper.findAll('.row.dir').find(row => row.text().includes('表紙'))!
    await coverDir.trigger('click')
    const imgRow = wrapper.findAll('.row').find(row => row.text().includes('cover_main.jpg'))!
    await imgRow.trigger('click')
    expect(store.set).toBe(0)
    expect(store.sel).toBe('表紙/cover_main.jpg')

    // Back to the root: a text row opens READ.
    await wrapper.get('.row.dir').trigger('click')
    const txtRow = wrapper.findAll('.row').find(row => row.text().includes('はじめにお読みください.txt'))!
    await txtRow.trigger('click')
    expect(store.leaf).toBe('script')
    expect(store.bayOpen).toBe(false) // the text pick closed the bottom drawer

    // An audio row, two levels deep, starts its track.
    await wrapper.findAll('.row.dir').find(row => row.text().includes('本編'))!.trigger('click')
    await wrapper.findAll('.row.dir').find(row => row.text().includes('mp3'))!.trigger('click')
    const mp3Row = wrapper.findAll('.row').find(row => row.text().includes('01_導入'))!
    await mp3Row.trigger('click')
    expect(player.trackIndex).toBe(0)

    // Lyrics in FILES are read; changing the playback pairing stays under LRC.
    await wrapper.get('.row.dir').trigger('click')
    await wrapper.findAll('.row.dir').find(row => row.text().includes('lyrics'))!.trigger('click')
    const lyricRow = wrapper.findAll('.row').find(row => row.text().includes('04_おやすみ.lrc'))!
    await lyricRow.trigger('click')
    expect(store.leaf).toBe('script')
    expect(store.activeTxt).toBe('本編/lyrics/04_おやすみ.lrc')
    expect(player.trackIndex).toBe(0)
    expect(store.pairs).toEqual({})
  })

  it('swaps the list for lyric files under the LRC chip and re-pairs the current track repeatedly', async () => {
    const { wrapper, store } = await mountBay()
    store.startTrack(1) // 02 has the automatic pairing
    await flushPromises()
    const before = store.lrcFor(1)
    const chips = wrapper.findAll('.chip')
    expect(chips.map(chip => chip.text())).toEqual(['MP3', 'WAV', 'LRC'])
    await chips[2].trigger('click')
    expect(store.lrcMode).toBe(true)
    expect(chips.map(chip => chip.attributes('aria-pressed'))).toEqual(['false', 'false', 'true'])
    expect(wrapper.findAll('.trk')).toHaveLength(2)
    const other = () => wrapper.findAll('.trk').find(row => row.attributes('aria-current') === undefined)!

    await other().trigger('click')
    expect(store.lrcFor(1)).not.toBe(before)
    expect(store.leaf).toBe('lrc')
    expect(store.track).toBe(1) // pairing never moves the playing track
    // Never locked: switch back.
    await other().trigger('click')
    expect(store.lrcFor(1)).toBe(before)

    // A format chip returns to the tracks, still on the same browsed format rules.
    await chips[1].trigger('click')
    expect(store.lrcMode).toBe(false)
    expect(store.fmt).toBe('WAV')
    expect(wrapper.findAll('.trk')).toHaveLength(8)
  })

  it('pairs an unpaired track by hand from the LRC list', async () => {
    const { wrapper, store } = await mountBay()
    store.startTrack(0) // 01 has none
    await flushPromises()
    expect(store.leaf).toBe('colophon')
    await wrapper.findAll('.chip')[2].trigger('click')
    await wrapper.findAll('.trk')[0].trigger('click')
    expect(store.pairs[0]).toBeDefined()
    expect(store.leaf).toBe('lrc')
  })

  describe('keyboard cursor', () => {
    const press = async (key: string) => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key }))
      await flushPromises()
    }
    const cursorOf = (wrapper: ReturnType<typeof mount>, row: string) => wrapper.findAll(row).findIndex(r => r.classes('cur'))

    it('starts on the playing track and follows it; Enter plays the row', async () => {
      const { wrapper, store } = await mountBay()
      store.dock = 'right'
      store.openBayTab('tracks')
      await flushPromises()
      expect(cursorOf(wrapper, '.trk')).toBe(0) // nothing of this work plays: row 0

      store.startTrack(2)
      await flushPromises()
      expect(cursorOf(wrapper, '.trk')).toBe(2)

      await press('ArrowDown')
      await press('Enter')
      expect(usePlayerStore().trackIndex).toBe(3)
      expect(cursorOf(wrapper, '.trk')).toBe(3)
    })

    it('starts on the paired lyric file in LRC mode, and Enter pairs the selected one', async () => {
      const { wrapper, store } = await mountBay()
      store.dock = 'right'
      store.startTrack(1)
      store.pairLrc(1, store.lrcFiles[1].rel_path)
      store.showLrc()
      await flushPromises()
      expect(cursorOf(wrapper, '.trk')).toBe(1)

      await press('ArrowUp')
      await press('Enter')
      expect(store.pairs[1]).toBe(store.lrcFiles[0].rel_path)
    })

    it('walks the folders: Enter goes in, Backspace goes up, the cursor resets', async () => {
      const { wrapper, store } = await mountBay()
      store.dock = 'right'
      store.openBayTab('files')
      await flushPromises()
      const dirs = wrapper.findAll('.row.dir')
      const target = dirs.findIndex(row => row.text().includes('表紙'))

      await press('Backspace') // the root has no parent
      expect(store.cwd).toEqual([])
      for (let i = 0; i < target; i++) await press('ArrowDown')
      await press('Enter')
      expect(store.cwd).toEqual(['表紙'])
      expect(cursorOf(wrapper, '.row')).toBe(0)

      await press('ArrowDown') // the first file under the ".." row
      await press('Enter')
      expect(store.sel).toBe(store.cwdDir!.children[0].rel_path)

      await press('Backspace')
      expect(store.cwd).toEqual([])
      expect(cursorOf(wrapper, '.row')).toBe(0)
    })

    it('enter on the ".." row goes up', async () => {
      const { store } = await mountBay()
      store.dock = 'right'
      store.openBayTab('files')
      store.selectDir(['表紙'])
      await flushPromises()
      await press('Enter')
      expect(store.cwd).toEqual([])
    })

    it('is inactive while the bottom drawer is shut', async () => {
      const { store } = await mountBay()
      store.openBayTab('tracks')
      store.closeBay()
      await flushPromises()
      await press('Enter')
      expect(usePlayerStore().rj).toBeNull()
      store.openBayTab('tracks')
      await flushPromises()
      await press('Enter')
      expect(usePlayerStore().trackIndex).toBe(0)
      expect(usePlayerStore().rj).toBe(RJ)
    })
  })
})
