import { computed, reactive } from 'vue'
import type { TreeNode } from '@/lib/api/types'

/**
 * Render/selection model over the server folder tree.
 *
 * The server JSON is the single source of truth: node names are basenames
 * (echoed verbatim), and full paths are only stored for payload use — this
 * module never derives names from paths and never normalizes any path.
 */

export type DirSelection = 'checked' | 'indeterminate' | 'unchecked'

export interface TreeModelNode {
  /** Stable identity — the server-provided full path, never rendered. */
  id: string
  /** Basename only, echoed verbatim from the server. */
  name: string
  /** Full path, echoed verbatim; used only for display. */
  path: string
  /**
   * Path relative to the member root — the identity file management
   * addresses ("sub/song.mp3"). The absolute path is never sent: the backend
   * resolves every item against the member it was told about.
   */
  relPath: string
  type: 'dir' | 'file'
  format: string
  bitrate: number | null
  size: number | null
  depth: number
  children: TreeModelNode[]
}

export interface TreeModel {
  root: TreeModelNode
  /** Expanded directories, by member-relative path ('' is the member root). */
  expanded: Set<string>
  /** Selected items, files and directories alike, by member-relative path. */
  selected: Set<string>
  /** Depth-first visible rows below the member root; re-read after toggles. */
  getVisibleNodes(): TreeModelNode[]
  isExpanded(relPath: string): boolean
  /** A directory's tri-state: its own mark, a covered one, or a mark below it. */
  dirSelection(relPath: string): DirSelection
  /** A selected ancestor already stands for this item. */
  isCovered(relPath: string): boolean
  toggleDir(relPath: string): void
  /** Open every directory, or fold everything back to the member root. */
  setAllExpanded(open: boolean): void
  /** Selecting an item drops its descendants: the item stands for them. */
  select(relPath: string, selected: boolean): void
  /** The selected member-relative paths, in tree order. */
  selectedPaths(): string[]
  /** One node by its member-relative path, for the operation dialogs. */
  findByRelPath(relPath: string): TreeModelNode | null
}

/** What a rebuilt tree keeps from the one it replaces. */
export interface TreeModelCarry {
  expanded: Iterable<string>
  selected: Iterable<string>
}

/** Dirs first, stable within each group (server order is preserved). */
function dirsFirst(children: TreeNode[]): TreeNode[] {
  return [...children].sort((a, b) => (a.type === b.type ? 0 : a.type === 'dir' ? -1 : 1))
}

function buildNode(node: TreeNode, depth: number): TreeModelNode {
  const children = (node.children ?? []).length
    ? dirsFirst(node.children ?? []).map((child) => buildNode(child, depth + 1))
    : []
  return {
    id: node.path,
    name: node.name,
    path: node.path,
    relPath: node.rel_path,
    type: node.type,
    format: node.format ?? '',
    bitrate: node.bitrate ?? null,
    size: node.size ?? null,
    depth,
    children,
  }
}

/**
 * Both sets survive a rebuild, keyed by member-relative path: a refresh, a
 * media-facts event or a finished operation re-reads the same member and must
 * not collapse the folders the user opened. Entries that no longer exist drop.
 */
export function createTreeModel(rootNode: TreeNode, carry?: TreeModelCarry): TreeModel {
  const root = buildNode(rootNode, 0)
  const byRel = new Map<string, TreeModelNode>()
  const index = (node: TreeModelNode): void => {
    byRel.set(node.relPath, node)
    node.children.forEach(index)
  }
  index(root)

  // The root is expanded by default; nested directories start collapsed.
  const expanded = reactive(new Set<string>([root.relPath]))
  const selected = reactive(new Set<string>())
  for (const rel of carry?.expanded ?? []) if (byRel.get(rel)?.type === 'dir') expanded.add(rel)
  for (const rel of carry?.selected ?? []) if (rel !== root.relPath && byRel.has(rel)) selected.add(rel)

  const visibleNodes = computed(() => {
    const out: TreeModelNode[] = []
    const walk = (node: TreeModelNode): void => {
      if (node !== root) out.push(node)
      if (node.type === 'dir' && expanded.has(node.relPath)) {
        for (const child of node.children) walk(child)
      }
    }
    walk(root)
    return out
  })

  function isCovered(relPath: string): boolean {
    for (const rel of selected) {
      if (rel !== relPath && relPath.startsWith(`${rel}/`)) return true
    }
    return false
  }

  function dirSelection(relPath: string): DirSelection {
    if (selected.has(relPath) || isCovered(relPath)) return 'checked'
    const below = relPath ? `${relPath}/` : ''
    for (const rel of selected) {
      if (rel.startsWith(below)) return 'indeterminate'
    }
    return 'unchecked'
  }

  function select(relPath: string, on: boolean): void {
    if (!on) {
      selected.delete(relPath)
      return
    }
    if (relPath === root.relPath || isCovered(relPath)) return
    for (const rel of [...selected]) if (rel.startsWith(`${relPath}/`)) selected.delete(rel)
    selected.add(relPath)
  }

  /** The selected paths in tree order, so a batch is applied top-down. */
  function selectedPaths(): string[] {
    const out: string[] = []
    const walk = (node: TreeModelNode): void => {
      if (selected.has(node.relPath)) out.push(node.relPath)
      node.children.forEach(walk)
    }
    walk(root)
    return out
  }

  function toggleDir(relPath: string): void {
    if (expanded.has(relPath)) expanded.delete(relPath)
    else expanded.add(relPath)
  }

  function setAllExpanded(open: boolean): void {
    expanded.clear()
    expanded.add(root.relPath)
    if (!open) return
    for (const node of byRel.values()) if (node.type === 'dir') expanded.add(node.relPath)
  }

  return {
    root,
    expanded,
    selected,
    getVisibleNodes: () => visibleNodes.value,
    isExpanded: (relPath) => expanded.has(relPath),
    dirSelection,
    isCovered,
    toggleDir,
    setAllExpanded,
    select,
    selectedPaths,
    findByRelPath: (relPath) => byRel.get(relPath) ?? null,
  }
}
