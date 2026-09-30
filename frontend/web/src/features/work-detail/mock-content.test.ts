import { describe, expect, it } from 'vitest'
import { deriveTracks, imageSets, parseLrc, parseTxt } from './derive'
import { loadText, loadWorkContent, imageUrl } from './mock-content'
import type { WorkContent, WorkContentCopy, WorkContentDir } from '@/lib/api/types'

/**
 * The mock is the content contract's reference implementation (draft spec §1):
 * any rj answers the same manifest, raw text never pre-structured structures.
 * These assertions are what Phase C's API answers have to keep true.
 */

const RJ = 'RJ01000001'

/** The mock is always online: this narrows the optional tree back for the reads below. */
function onlineCopy(content: WorkContent): WorkContentCopy & { tree: WorkContentDir } {
  const copy = content.copies.find((entry) => entry.online)
  expect(copy).toBeDefined()
  expect(copy?.tree).toBeDefined()
  return copy as WorkContentCopy & { tree: WorkContentDir }
}

describe('mock content', () => {
  it('answers any rj with the contract shape and echoes it back', async () => {
    const content = await loadWorkContent('RJ99999999')
    expect(content.rj).toBe('RJ99999999')
    const online = onlineCopy(content)
    expect(online.tree.children.length).toBeGreaterThan(0)
  })

  it('records scan-time dimensions and durations on every image and audio file', async () => {
    const content = await loadWorkContent(RJ)
    const files: unknown[] = []
    const walk = (dir: WorkContentDir): void => {
      for (const child of dir.children) {
        if ('children' in child) walk(child)
        else files.push(child)
      }
    }
    walk(onlineCopy(content).tree)
    for (const file of files) {
      const item = file as { kind: string; width: number | null; height: number | null; duration_sec: number | null }
      if (item.kind === 'img') expect([item.width, item.height]).not.toContain(null)
      if (item.kind === 'aud') expect(item.duration_sec).not.toBeNull()
    }
  })

  it('serves raw LRC and TXT text that the real parsers read', async () => {
    const content = await loadWorkContent(RJ)
    const copy = onlineCopy(content)
    const tracks = deriveTracks(copy.tree)
    const paired = tracks.find((track) => track.lrcRel !== null)
    expect(paired?.lrcRel).toBeDefined()
    const lrcText = await loadText({ rj: RJ, copyId: copy.copy_id, relPath: paired!.lrcRel! })
    expect(lrcText).toMatch(/^\[\d{2}:\d{2}\.\d{2}\]/m)
    expect(parseLrc(lrcText).length).toBeGreaterThan(5)

    const script = '台本/台本_全編.txt'
    const text = await loadText({ rj: RJ, copyId: copy.copy_id, relPath: script })
    expect(text).toContain('【')
    const lines = parseTxt(text)
    expect(lines.filter((line) => line.kind === 'sp').length).toBeGreaterThan(20)
    expect(lines.filter((line) => line.kind === 'scene').length).toBe(4)
  })

  it('produces a cover set first and a track that exists in one format only', async () => {
    const content = await loadWorkContent(RJ)
    const copy = onlineCopy(content)
    const sets = imageSets(copy.tree)
    expect(sets[0].id).toBe('表紙')
    expect(sets[0].cover).toBe(true)
    const tracks = deriveTracks(copy.tree)
    expect(tracks).toHaveLength(8)
    expect(tracks[7].formats).toEqual(['WAV'])
  })

  it('gives every image a pixel source', async () => {
    const content = await loadWorkContent(RJ)
    const copy = onlineCopy(content)
    const sets = imageSets(copy.tree)
    const first = sets[0].files[0]
    expect(imageUrl({ rj: RJ, copyId: copy.copy_id, relPath: first.rel_path })).toMatch(/^data:image\/svg\+xml/)
  })
})
