import { describe, expect, it } from 'vitest'
import { QueryObserver } from '@tanstack/vue-query'
import type { Library, LibraryDir, TreeNode } from '@/lib/api/types'
import { queryKeys } from '@/queries/query-keys'
import { createTestQueryClient } from '@/test/query-client'
import { syncAfterScan } from './use-library-scan'

const library: Library = {
  id: 'lib-a',
  name: 'Archive',
  root_path: 'D:\\Music',
  created_at: '',
  updated_at: '',
  last_scan_at: null,
  last_scan_status: '',
  last_scan_error: '',
}

const dirs: LibraryDir[] = [
  { name: 'Alpha', path: 'D:\\Music\\Alpha', rel_path: 'Alpha', dir_id: 'dir-alpha', audio_file_count: 1, file_count: 1, added_at: '2026-05-01T00:00:00Z', work_code: null },
]

const tree: TreeNode = {
  name: 'Alpha',
  path: 'D:\\Music\\Alpha',
  rel_path: 'Alpha',
  type: 'dir',
  format: '',
  bitrate: null,
  children: [],
}

// A seeded workset feed entry standing in for the workset domain caches.
const seededWorkset = {
  workset_id: 'ws-1',
  title: 't',
  version: 1,
  library: null,
  planning_state: 'planned',
  current_revision: null,
  active_generation: null,
  latest_generation: null,
  members: [],
  updated_at: '',
  created_at: '',
}

function seedLibraryDomain(client: ReturnType<typeof createTestQueryClient>) {
  client.setQueryData(queryKeys.libraries.list(), [library])
  for (const key of [queryKeys.works.content('RJ01000001'), queryKeys.works.detail('RJ01000001'), queryKeys.works.list({})]) client.setQueryData(key, 'cached')
  client.setQueryData(queryKeys.libraries.dirs('lib-a', 'd:/music'), dirs)
  client.setQueryData(queryKeys.libraries.memberTree('lib-a', 'd:/music', 'folder-a'), tree)
}

function seedWorksetDomain(client: ReturnType<typeof createTestQueryClient>) {
  client.setQueryData([...queryKeys.worksets.list(null), 'infinite'], {
    pages: [{ worksets: [seededWorkset], next_cursor: undefined }],
    pageParams: [undefined],
  })
}

function feedUntouched(client: ReturnType<typeof createTestQueryClient>): boolean {
  return client.getQueryData([...queryKeys.worksets.list(null), 'infinite']) !== undefined
}

describe('syncAfterScan terminal cache matrix', () => {
  it('completed refreshes library metadata, derived caches, and the workset domain', async () => {
    const client = createTestQueryClient()
    seedLibraryDomain(client)
    seedWorksetDomain(client)
    const foldersKey = queryKeys.libraries.dirs('lib-a', 'd:/music')
    const treeKey = queryKeys.libraries.memberTree('lib-a', 'd:/music', 'folder-a')

    await syncAfterScan(client, 'lib-a', 'completed', 'event')

    // Inactive derived entries are dropped…
    expect(client.getQueryData(foldersKey)).toBeUndefined()
    expect(client.getQueryData(treeKey)).toBeUndefined()
    // …the library list is invalidated for metadata refresh…
    expect(client.getQueryState(queryKeys.libraries.list())?.isInvalidated).toBe(true)
    // …and workset validation caches are conservatively dropped (stale is
    // derived from the live inventory, which a completed scan just changed).
    expect(feedUntouched(client)).toBe(false)
    for (const key of [queryKeys.works.content('RJ01000001'), queryKeys.works.detail('RJ01000001'), queryKeys.works.list({})]) expect(client.getQueryData(key)).toBeUndefined()
  })

  it('completed updates an actively viewed work list, detail and manifest', async () => {
    const client = createTestQueryClient()
    const keys = [queryKeys.works.list({}), queryKeys.works.detail('RJ01000001'), queryKeys.works.content('RJ01000001')]
    const stops = keys.map((key) => {
      client.setQueryData(key, 'old inventory')
      return new QueryObserver(client, { queryKey: key, queryFn: async () => 'new inventory' }).subscribe(() => {})
    })
    await syncAfterScan(client, 'lib-a', 'completed', 'event')
    for (const key of keys) expect(client.getQueryData(key)).toBe('new inventory')
    for (const stop of stops) stop()
    client.clear()
  })

  it('confirmed cancel/error over SSE only refreshes library metadata', async () => {
    const client = createTestQueryClient()
    seedLibraryDomain(client)
    seedWorksetDomain(client)
    const foldersKey = queryKeys.libraries.dirs('lib-a', 'd:/music')
    const treeKey = queryKeys.libraries.memberTree('lib-a', 'd:/music', 'folder-a')

    await syncAfterScan(client, 'lib-a', 'cancelled', 'event')

    expect(client.getQueryData(foldersKey)).toEqual(dirs)
    expect(client.getQueryData(treeKey)).toEqual(tree)
    expect(client.getQueryState(queryKeys.libraries.list())?.isInvalidated).toBe(true)
    // Nothing was committed: the workset domain keeps its caches.
    expect(feedUntouched(client)).toBe(true)
    expect(client.getQueryData(queryKeys.works.content('RJ01000001'))).toBe('cached')
  })

  it('transport failure conservatively refreshes derived caches and worksets', async () => {
    const client = createTestQueryClient()
    seedLibraryDomain(client)
    seedWorksetDomain(client)
    const foldersKey = queryKeys.libraries.dirs('lib-a', 'd:/music')
    const treeKey = queryKeys.libraries.memberTree('lib-a', 'd:/music', 'folder-a')

    await syncAfterScan(client, 'lib-a', 'error', 'transport')

    expect(client.getQueryData(foldersKey)).toBeUndefined()
    expect(client.getQueryData(treeKey)).toBeUndefined()
    expect(client.getQueryState(queryKeys.libraries.list())?.isInvalidated).toBe(true)
    expect(feedUntouched(client)).toBe(false)
    for (const key of [queryKeys.works.content('RJ01000001'), queryKeys.works.detail('RJ01000001'), queryKeys.works.list({})]) expect(client.getQueryData(key)).toBeUndefined()
  })

  it('transport failure before the first SSE event skips the derived refresh', async () => {
    const client = createTestQueryClient()
    seedLibraryDomain(client)
    seedWorksetDomain(client)
    const foldersKey = queryKeys.libraries.dirs('lib-a', 'd:/music')
    const treeKey = queryKeys.libraries.memberTree('lib-a', 'd:/music', 'folder-a')

    // The POST /scans request was rejected before any event arrived: the
    // scan never started, so folders/trees cannot have changed.
    await syncAfterScan(client, 'lib-a', 'error', 'transport', false)

    expect(client.getQueryData(foldersKey)).toEqual(dirs)
    expect(client.getQueryData(treeKey)).toEqual(tree)
    expect(client.getQueryState(queryKeys.libraries.list())?.isInvalidated).toBe(true)
    expect(feedUntouched(client)).toBe(true)
    expect(client.getQueryData(queryKeys.works.content('RJ01000001'))).toBe('cached')
  })

  it('user cancel is metadata-only even when the stream had started', async () => {
    const client = createTestQueryClient()
    seedLibraryDomain(client)
    seedWorksetDomain(client)
    const foldersKey = queryKeys.libraries.dirs('lib-a', 'd:/music')
    const treeKey = queryKeys.libraries.memberTree('lib-a', 'd:/music', 'folder-a')

    // Cancel aborts the stream (transport terminal) after a 'started' event:
    // materialized folders are replaced in one transaction only on completion,
    // so nothing changed — the derived caches must not be dropped or refetched.
    await syncAfterScan(client, 'lib-a', 'cancelled', 'transport', true)

    expect(client.getQueryData(foldersKey)).toEqual(dirs)
    expect(client.getQueryData(treeKey)).toEqual(tree)
    expect(client.getQueryState(queryKeys.libraries.list())?.isInvalidated).toBe(true)
    expect(feedUntouched(client)).toBe(true)
    expect(client.getQueryData(queryKeys.works.content('RJ01000001'))).toBe('cached')
  })
})
