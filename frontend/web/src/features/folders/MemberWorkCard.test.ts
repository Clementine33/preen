import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { apiClientKey } from '@/lib/api/client'
import { desktopAdapterKey } from '@/lib/desktop/desktop-adapter'
import { useLibraryUiStore } from '@/stores/library-ui'
import { apiStub } from '@/test/api-stub'
import { installTestQueryPlugin } from '@/test/query-client'
import { makeWorkDetail } from '@/test/work-fixture'
import MemberWorkCard from './MemberWorkCard.vue'

const hostAdapter = {
  platform: 'browser',
  lyricsWindow: false,
  openLyricsWindow: async () => null,
  resolveApi: async () => ({ baseUrl: '', token: null }),
}

async function mountCard(detail: ReturnType<typeof makeWorkDetail> | null, veil = false, adapter: object = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useLibraryUiStore().veil = veil
  const api = apiStub({
    getWork: vi.fn(async () => {
      if (!detail)
        throw new Error('404')
      return detail
    }) as never,
    workFileUrl: (() => '/cover.jpg') as never,
  })
  const wrapper = mount(MemberWorkCard, {
    props: { rj: 'RJ01000001', path: '/lib/RJ01000001' },
    global: {
      plugins: [
        pinia,
        installTestQueryPlugin(),
      ],
      provide: {
        [apiClientKey as symbol]: api,
        [desktopAdapterKey as symbol]: {
          ...hostAdapter,
          ...adapter,
        },
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('memberWorkCard', () => {
  it('shows cover, title, circle and voice actors', async () => {
    const wrapper = await mountCard(
      makeWorkDetail({
        nsfw: false,
        local_cover: {
          copy_id: 'c',
          rel_path: 'cover.jpg',
          version: '1',
          thumb_v: 't',
        },
      }),
    )
    expect(wrapper.get('img').attributes('src')).toBe('/cover.jpg')
    expect(wrapper.get('[data-testid=work-title]').text()).toBe('雨宿りの縁側で、君と眠るまで')
    expect(wrapper.get('[data-testid=work-circle]').text()).toBe('mete0r')
    expect(wrapper.get('[data-testid=work-voice]').text()).toBe('伊ヶ崎綾香')
  })

  it('keeps an R18 cover hatched while the veil is on', async () => {
    const wrapper = await mountCard(
      makeWorkDetail({
        nsfw: true,
        local_cover: {
          copy_id: 'c',
          rel_path: 'cover.jpg',
          version: '1',
          thumb_v: 't',
        },
      }),
      true,
    )
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('opens the copy folder only when the host can', async () => {
    expect((await mountCard(makeWorkDetail())).find('[data-testid=work-path-open]').exists()).toBe(false)
    const openFolder = vi.fn(async () => {})
    const wrapper = await mountCard(makeWorkDetail(), false, { openFolder })
    await wrapper.get('[data-testid=work-path-open]').trigger('click')
    expect(openFolder).toHaveBeenCalledWith('/lib/RJ01000001')
  })

  it('renders nothing when the work is gone', async () => {
    const wrapper = await mountCard(null)
    expect(wrapper.find('[data-testid=member-work-card]').exists()).toBe(false)
  })
})
