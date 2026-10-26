import { useQueryClient, type InfiniteData } from '@tanstack/vue-query'
import { toValue, type MaybeRefOrGetter } from 'vue'
import { useToast } from '@/composables/use-toast'
import { ApiError, useApiClient } from '@/lib/api/client'
import type { CollectionDetail, WorkSummary, WorksSort } from '@/lib/api/types'
import { COLLECTION_CHUNK, useCollectionMemberWrites } from '@/queries/collections'
import { queryKeys } from '@/queries/query-keys'
import { landingIndex, moveIntent } from './move-intent'

// Editing the members of the list on screen: moves and removals are applied to
// the cached chunks at once, then written; the member writes re-read the
// collection afterwards, so a failure or a conflicting edit settles on the
// server's truth. `gap` is a screen gap (see move-intent.ts).
export function useCollectionEdit(src: {
  id: MaybeRefOrGetter<string>
  sort: MaybeRefOrGetter<WorksSort | ''>
  direction: MaybeRefOrGetter<'asc' | 'desc'>
  name: MaybeRefOrGetter<string>
  works: MaybeRefOrGetter<WorkSummary[]>
  hasMore: MaybeRefOrGetter<boolean>
}) {
  const api = useApiClient()
  const client = useQueryClient()
  const writes = useCollectionMemberWrites()
  const toast = useToast()

  /** Rewrites the cached rows (re-chunked as the API pages them); `delta` adjusts the total. */
  function edit(change: (rows: WorkSummary[]) => WorkSummary[], delta = 0) {
    const key = queryKeys.collections.pages(toValue(src.id), toValue(src.sort), toValue(src.direction))
    void client.cancelQueries({ queryKey: key })
    client.setQueryData<InfiniteData<CollectionDetail, number>>(key, (old) => {
      if (!old) return old
      const rows = change(old.pages.flatMap((p) => p.works))
      return {
        ...old,
        pages: old.pages.map((p, i) => ({
          ...p, total: p.total + delta, work_count: p.work_count + delta,
          works: rows.slice(i * COLLECTION_CHUNK, i === old.pages.length - 1 ? undefined : (i + 1) * COLLECTION_CHUNK),
        })),
      }
    })
  }

  function fail(e: unknown) {
    if (e instanceof ApiError && e.code === 'COLLECTION_MEMBER_CONFLICT') toast.show({ text: '合集已在别处变动，已刷新' })
    else toast.error(e)
  }

  async function move(rj: string, gap: number): Promise<void> {
    const rows = toValue(src.works)
    const from = rows.findIndex((w) => w.rj === rj)
    const direction = toValue(src.direction)
    const intent = from < 0 ? null : moveIntent(rows.map((w) => w.rj), from, gap, direction, !toValue(src.hasMore))
    if (!intent) return
    const id = toValue(src.id)
    edit((all) => {
      const moved = all.splice(from, 1)[0]!
      all.splice(landingIndex(from, gap), 0, moved)
      return all
    })
    try {
      let before = intent.before
      // ponytail: one extra read when the saved order's first member is not loaded.
      if (before === 'first-saved') before = (await api.getCollection(id, { limit: 1, page: 1, direction: 'asc' })).works[0]?.rj ?? null
      await writes.move(id, rj, before)
    } catch (e) { fail(e) }
  }

  async function remove(rj: string): Promise<void> {
    const rows = toValue(src.works)
    const i = rows.findIndex((w) => w.rj === rj)
    if (i < 0) return
    const [id, name, custom] = [toValue(src.id), toValue(src.name), toValue(src.sort) === '']
    // The saved-order neighbour after it: the next row ascending, the one above descending.
    const neighbour = !custom ? null : (toValue(src.direction) === 'asc' ? rows[i + 1] : rows[i - 1])?.rj ?? null
    edit((all) => all.filter((w) => w.rj !== rj), -1)
    try {
      await writes.remove(id, rj)
    } catch (e) { return fail(e) }
    toast.show({
      text: custom ? `已从 ${name} 移除` : `已从 ${name} 移除（撤销将放回末尾）`,
      action: {
        label: '撤销',
        run: () => void (async () => {
          try {
            await writes.append(id, [rj])
            if (neighbour) await writes.move(id, rj, neighbour).catch((e) => { if (!(e instanceof ApiError && e.code === 'COLLECTION_MEMBER_CONFLICT')) throw e })
          } catch (e) { fail(e) }
        })(),
      },
    })
  }

  return { move, remove }
}
