import { describe, expect, it } from 'vitest'
import type { TreeNode } from '@/lib/api/types'
import { createTreeModel } from './tree-model'

function dir(name: string, path: string, children: TreeNode[] = []): TreeNode {
  return { name, path, rel_path: relOf(path), type: 'dir', format: '', bitrate: null, children }
}

function file(name: string, path: string, format = 'flac', bitrate = 920000, size = 12345): TreeNode {
  return { name, path, rel_path: relOf(path), type: 'file', format, bitrate, size }
}

/** The member-relative path a fixture node would carry from the server. */
function relOf(path: string): string {
  return path.startsWith('/music/albumA/') ? path.slice('/music/albumA/'.length) : ''
}

const rootFixture: TreeNode = dir('albumA', '/music/albumA', [
  file('track1.flac', '/music/albumA/track1.flac'),
  dir('sub2', '/music/albumA/sub2', [file('b.flac', '/music/albumA/sub2/b.flac')]),
  file('track2.flac', '/music/albumA/track2.flac'),
  dir('sub1', '/music/albumA/sub1', [
    file('a.flac', '/music/albumA/sub1/a.flac'),
    dir('deep', '/music/albumA/sub1/deep', [file('c.flac', '/music/albumA/sub1/deep/c.flac')]),
  ]),
])

describe('tree model', () => {
  it('orders children dirs-first while preserving server order within each group', () => {
    const model = createTreeModel(rootFixture)
    expect(model.root.children.map((node) => node.name)).toEqual([
      'sub2',
      'sub1',
      'track1.flac',
      'track2.flac',
    ])
  })

  it('lists the rows below the member root, collapsed by default', () => {
    const model = createTreeModel(rootFixture)
    expect(model.root.depth).toBe(0)
    expect(model.getVisibleNodes().map((node) => node.name)).toEqual(['sub2', 'sub1', 'track1.flac', 'track2.flac'])
    expect(model.getVisibleNodes().find((node) => node.name === 'sub1')?.depth).toBe(1)
  })

  it('expands and collapses a directory via toggle', () => {
    const model = createTreeModel(rootFixture)
    model.toggleDir('sub1')
    expect(model.getVisibleNodes().map((node) => node.name)).toContain('a.flac')
    model.toggleDir('sub1')
    expect(model.getVisibleNodes().map((node) => node.name)).not.toContain('a.flac')
  })

  it('expands and folds every directory at once', () => {
    const model = createTreeModel(rootFixture)
    model.setAllExpanded(true)
    expect(model.getVisibleNodes().map((node) => node.name)).toContain('c.flac')
    model.setAllExpanded(false)
    expect(model.getVisibleNodes().map((node) => node.name)).toEqual(['sub2', 'sub1', 'track1.flac', 'track2.flac'])
  })

  it('selecting a directory selects the directory itself and covers what is below', () => {
    const model = createTreeModel(rootFixture)

    model.select('sub1', true)

    expect([...model.selected]).toEqual(['sub1'])
    expect(model.selectedPaths()).toEqual(['sub1'])
    expect(model.dirSelection('sub1')).toBe('checked')
    expect(model.dirSelection('sub1/deep')).toBe('checked')
    expect(model.isCovered('sub1/a.flac')).toBe(true)
    expect(model.isCovered('sub1')).toBe(false)
    expect(model.dirSelection('')).toBe('indeterminate')

    model.select('sub1', false)
    expect(model.selected.size).toBe(0)
    expect(model.dirSelection('sub1')).toBe('unchecked')
  })

  it('selecting a directory replaces marks already set below it, and covered items refuse a mark', () => {
    const model = createTreeModel(rootFixture)
    model.select('sub1/a.flac', true)
    expect(model.dirSelection('sub1')).toBe('indeterminate')
    model.select('sub1', true)
    expect(model.selectedPaths()).toEqual(['sub1'])
    model.select('sub1/deep/c.flac', true)
    expect(model.selectedPaths()).toEqual(['sub1'])
  })

  it('can select an empty directory, never the member root', () => {
    const model = createTreeModel(dir('albumA', '/music/albumA', [dir('empty', '/music/albumA/empty')]))
    model.select('empty', true)
    model.select('', true)
    expect(model.selectedPaths()).toEqual(['empty'])
  })

  it('keeps expansion and selection across a rebuild, dropping what no longer exists', () => {
    const model = createTreeModel(rootFixture)
    model.toggleDir('sub1')
    model.toggleDir('sub2')
    model.select('track1.flac', true)
    model.select('sub2', true)

    const next = createTreeModel(
      dir('albumA', '/music/albumA', [
        file('track1.flac', '/music/albumA/track1.flac'),
        dir('sub1', '/music/albumA/sub1', [file('a.flac', '/music/albumA/sub1/a.flac')]),
      ]),
      model,
    )
    expect(next.isExpanded('sub1')).toBe(true)
    expect(next.isExpanded('sub2')).toBe(false)
    expect(next.selectedPaths()).toEqual(['track1.flac'])
  })

  it('uses server basenames as labels and never synthesizes names from paths', () => {
    const model = createTreeModel(rootFixture)
    expect(model.root.name).toBe('albumA')
    for (const node of model.getVisibleNodes()) {
      expect(node.name).not.toContain('/')
      expect(node.name).not.toContain('\\')
    }
  })
})
