import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { apiClientKey } from '@/lib/api/client'
import type { WorkSummary } from '@/lib/api/types'
import { bumpCoverEpoch } from '@/queries/cover-epoch'
import { apiStub } from '@/test/api-stub'
import WorkCard from './WorkCard.vue'

/**
 * The wall shows the cover already on disk through the backend, and the
 * scraped remote link only while the local file cannot be read.
 */

const REMOTE = 'https://covers.test/RJ01000001_sam.jpg'

function work(version = '1-5-9'): WorkSummary {
  return {
    rj: 'RJ01000001',
    title: 'Title',
    thumbnail_url: REMOTE,
    circle: null,
    language: null,
    nsfw: null,
    age_category: null,
    release_date: null,
    dl_count: null,
    rating: null,
    favorite: false,
    user_tags: [],
    has_note: false,
    collected_at: '2026-01-01T00:00:00Z',
    copy_count: 1,
    local_cover: { copy_id: 'copy-1', rel_path: 'cover.jpg', version },
  }
}

function mountCard(summary: WorkSummary) {
  const api = apiStub({
    workFileUrl: ((ref: { copyId: string; relPath: string }, version?: string) =>
      `/files/${ref.copyId}/${ref.relPath}?v=${version}`) as never,
  })
  return mount(WorkCard, {
    props: { work: summary, selected: false, playing: false },
    global: { provide: { [apiClientKey as symbol]: api } },
  })
}

afterEach(() => bumpCoverEpoch())

describe('WorkCard cover', () => {
  it('shows the local cover ahead of the remote link, versioned by its address', async () => {
    const wrapper = mountCard(work())
    expect(wrapper.get('img').attributes('src')).toBe('/files/copy-1/cover.jpg?v=1-5-9')
    await wrapper.setProps({ work: work('2-6-9') })
    expect(wrapper.get('img').attributes('src')).toBe('/files/copy-1/cover.jpg?v=2-6-9')
  })

  it('falls back to the remote link when the local file fails to load', async () => {
    const wrapper = mountCard(work())
    await wrapper.get('img').trigger('error')
    expect(wrapper.get('img').attributes('src')).toBe(REMOTE)
  })

  it('retries the same local address once a later read lands', async () => {
    const wrapper = mountCard(work())
    await wrapper.get('img').trigger('error')
    expect(wrapper.get('img').attributes('src')).toBe(REMOTE)
    bumpCoverEpoch() // a list/detail refresh landed; address and version unchanged
    await nextTick()
    expect(wrapper.get('img').attributes('src')).toBe('/files/copy-1/cover.jpg?v=1-5-9')
  })

  it('keeps the remote link for a work without a local cover', () => {
    const wrapper = mountCard({ ...work(), local_cover: null })
    expect(wrapper.get('img').attributes('src')).toBe(REMOTE)
  })
})
