import type { WorkContentDir, WorkContentFile, WorkDetail } from '@/lib/api/types'

/** A real file object of a manifest tree, addressed by rel_path. */
export function findFile(tree: WorkContentDir, relPath: string): WorkContentFile {
  const walk = (dir: WorkContentDir): WorkContentFile | null => {
    for (const child of dir.children) {
      if ('children' in child) {
        const hit = walk(child)
        if (hit) return hit
      } else if (child.rel_path === relPath) return child
    }
    return null
  }
  const hit = walk(tree)
  if (hit === null) throw new Error(`no such file: ${relPath}`)
  return hit
}

/** A full work detail with every field set, for detail-page tests to override. */
export function makeWorkDetail(overrides: Partial<WorkDetail> = {}): WorkDetail {
  return {
    rj: 'RJ01000001',
    title: '雨宿りの縁側で、君と眠るまで',
    thumbnail_url: null,
    circle: 'mete0r',
    language: 'JPN',
    nsfw: true,
    release_date: '2024-03-08',
    dl_count: 12_480,
    rating: 8,
    favorite: true,
    user_tags: [{ id: 't-1', name: '睡前' }],
    has_note: true,
    collected_at: '2026-01-14T00:00:00Z',
    copy_count: 1,
    source_url: 'https://www.dlsite.com/maniax/work/=/product_id/RJ01000001.html',
    cover_url: null,
    sample_cover_url: null,
    series_id: null,
    series_name: '雨宿りシリーズ',
    voice_actors: [{ id: 'va-1', source: 'dlsite', source_id: '1', name: '伊ヶ崎綾香' }],
    source_tags: [{ id: 'st-1', source: 'dlsite', source_id: 'tag1', name: '耳かき' }],
    note: '',
    language_editions: [
      { version_root_rj: 'RJ01000001', language: 'ja', label: '日本語', edition_id: 'e-1', display_order: 0 },
    ],
    relation: { role: null, language: null, original_rj: null, parent_rj: null },
    copies: [{ copy_id: 'copy-a', library_id: 'lib-a', rel_path: 'works/RJ01000001' }],
    placeholder: false,
    ...overrides,
  }
}
