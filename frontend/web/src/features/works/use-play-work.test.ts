import type { WorkContent, WorkContentFile, WorkListening } from '@/lib/api/types'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import { usePlayerStore } from '@/stores/player'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import { usePlayWork } from './use-play-work'

/**
 * Playing a work from the wall continues its Listening Position: the newest
 * row with a position whose track still exists, in the track's own format when
 * the preferred one is missing there, and at the saved second.
 */

const RJ = 'RJ01000001'
function audio(relPath: string, format: string): WorkContentFile {
  return {
    name: relPath,
    rel_path: relPath,
    kind: 'aud',
    size: 0,
    format,
    bitrate: null,
    width: null,
    height: null,
    duration_sec: 60,
    thumb_v: null,
  }
}
// 01 ships both codecs, 02 is WAV-only; the preference below is MP3.
const CONTENT: WorkContent = {
  rj: RJ,
  copies: [
    {
      copy_id: 'copy-a',
      library_id: 'lib-a',
      dir_id: 'dir-a',
      online: true,
      tree: {
        name: '',
        rel_path: '',
        children: [
          audio('01.mp3', 'MP3'),
          audio('01.wav', 'WAV'),
          audio('02.wav', 'WAV'),
          audio('03.mp3', 'MP3'),
        ],
      },
    },
  ],
}
function row(extra: Partial<WorkListening>): WorkListening {
  return {
    rendition: '',
    track_key: '02',
    position_sec: 125,
    track_index: 1,
    track_count: 3,
    finished_at: null,
    updated_at: '2026-10-01T00:00:00.000000000Z',
    ...extra,
  }
}

async function playFromWall(listening: WorkListening[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const api = apiStub({
    getWork: vi.fn().mockResolvedValue(makeWorkDetail({ rj: RJ, listening })),
    getWorkContent: vi.fn().mockResolvedValue(CONTENT),
    getPreferences: vi.fn().mockResolvedValue({
      track_formats: ['MP3'],
    }),
  })
  let playWork!: ReturnType<typeof usePlayWork>['playWork']
  mount(defineComponent({
    setup: () => { playWork = usePlayWork().playWork; return () => null },
  }), {
    global: {
      plugins: [
        pinia,
        installTestQueryPlugin(),
      ],
      provide: { [apiClientKey as symbol]: api },
    },
  })
  const player = usePlayerStore()
  const play = vi.spyOn(player, 'play')
  await flushPromises() // the preference settles
  await playWork({ rj: RJ, title: 'T' })
  return { player, play }
}

beforeEach(() => {
  vi.stubGlobal('Audio', class extends EventTarget {
    play() { return Promise.resolve() }
    pause() {}
    load() {}
    removeAttribute() {}
  })
})

describe('usePlayWork, resuming', () => {
  it('plays a WAV-only saved track in WAV at its second under an MP3 preference', async () => {
    const { player, play } = await playFromWall([
      row({}),
    ])
    const [rj, queue, index, at] = play.mock.calls[0]!
    expect(rj).toBe(RJ)
    expect(queue[index!]).toMatchObject({
      key: '02',
      source: { format: 'WAV' },
    })
    expect(at).toBe(125)
    expect(player.trackKey).toBe('02')
    expect(player.elapsed).toBe(125)
  })

  it('keeps the saved track in the preferred format when it has it', async () => {
    const { play } = await playFromWall([
      row({ track_key: '03', position_sec: 7 }),
    ])
    const [, queue, index, at] = play.mock.calls[0]!
    expect(queue[index!]).toMatchObject({
      key: '03',
      source: { format: 'MP3' },
    })
    expect(at).toBe(7)
  })

  it('skips a Finished or orphan newest row for the next one', async () => {
    const finished = row({
      track_key: null,
      position_sec: null,
      track_index: null,
      track_count: null,
      finished_at: '2026-10-05T00:00:00.000000000Z',
      updated_at: '2026-10-05T00:00:00.000000000Z',
    })
    const orphan = row({ rendition: 'gone', track_key: 'gone/01', updated_at: '2026-10-04T00:00:00.000000000Z' })
    const older = row({ track_key: '03', position_sec: 40, updated_at: '2026-09-01T00:00:00.000000000Z' })
    const { play } = await playFromWall([finished, orphan, older])
    const [, queue, index, at] = play.mock.calls[0]!
    expect(queue[index!]).toMatchObject({ key: '03' })
    expect(at).toBe(40)
  })

  it('starts at track 0, second 0, when the saved key is missing', async () => {
    const { play } = await playFromWall([
      row({ track_key: 'deleted' }),
    ])
    const [, queue, index, at] = play.mock.calls[0]!
    expect(queue[index!]).toMatchObject({
      key: '01',
      source: { format: 'MP3' },
    })
    expect(at).toBeUndefined()
  })
})
