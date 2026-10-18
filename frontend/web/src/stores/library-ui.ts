import { defineStore } from 'pinia'
import type { Library, LibraryDir } from '@/lib/api/types'

/**
 * UI-only store: the active library and the overview's directory selection.
 * Server-derived data lives in Vue Query; this store never holds libraries,
 * directories, loading, or error state.
 *
 * The selection is a set of library-relative paths, because that is the
 * identity a record is created from — a folder id from an earlier scan would
 * have been renumbered by the next one.
 */
const VEIL_KEY = 'preen.r18-veil'

function readVeil(): boolean {
  try {
    return localStorage.getItem(VEIL_KEY) === '1'
  } catch {
    return false
  }
}

export const useLibraryUiStore = defineStore('library-ui', {
  state: () => ({
    activeLibraryId: null as string | null,
    /**
     * The work wall's aggregate scope: every library at once. Only the wall
     * reads it; the active ID stays a real library for every other page.
     */
    allLibraries: false,
    selectedDirPaths: [] as string[],
    /**
     * The overview list's scroll offset, kept across a trip into a member's
     * files and back: leaving the list must not lose the reader's place.
     */
    overviewScroll: 0,
    /** The R18 viewer option, off by default; kept in localStorage, set on the wall, read by the wall and the detail page. */
    veil: readVeil(),
  }),
  actions: {
    setActiveLibrary(id: string) {
      this.allLibraries = false
      if (id === this.activeLibraryId) return
      this.activeLibraryId = id
      // Switching libraries clears the previous library's temporary
      // selection; a scroll offset of another list means nothing here either.
      this.selectedDirPaths = []
      this.overviewScroll = 0
    },
    showAllLibraries() {
      if (this.allLibraries) return
      this.allLibraries = true
      this.selectedDirPaths = []
      this.overviewScroll = 0
    },
    // Keeps the active ID valid against a freshly fetched library list: keep
    // the current ID when it still exists, otherwise fall back to the first
    // library (or null when there are none).
    reconcileLibraries(libraries: Library[]) {
      const next =
        this.activeLibraryId !== null && libraries.some((library) => library.id === this.activeLibraryId)
          ? this.activeLibraryId
          : (libraries[0]?.id ?? null)
      if (next !== this.activeLibraryId) {
        this.activeLibraryId = next
        this.selectedDirPaths = []
      }
    },
    // Drops selected paths that no longer exist in a successful listing.
    // Listings from another library are ignored, so a late response or a
    // background refetch can never mutate the current selection.
    reconcileDirs(libraryId: string, dirs: LibraryDir[]) {
      if (libraryId !== this.activeLibraryId) return
      if (this.selectedDirPaths.length === 0) return
      const paths = new Set(dirs.map((dir) => dir.rel_path))
      const next = this.selectedDirPaths.filter((path) => paths.has(path))
      if (next.length !== this.selectedDirPaths.length) this.selectedDirPaths = next
    },
    // The optional `dirs` argument restores the existence guard: a click that
    // lands after reconcileDirs dropped a path (a scan-sync refresh removed it
    // right before the event) must not push a stale path into the selection.
    toggleDir(relPath: string, dirs?: LibraryDir[]) {
      if (this.selectedDirPaths.includes(relPath)) {
        this.selectedDirPaths = this.selectedDirPaths.filter((selected) => selected !== relPath)
      } else {
        if (dirs && !dirs.some((dir) => dir.rel_path === relPath)) return
        this.selectedDirPaths.push(relPath)
      }
    },
    setDirSelected(relPath: string, selected: boolean, dirs?: LibraryDir[]) {
      if (selected !== this.selectedDirPaths.includes(relPath)) this.toggleDir(relPath, dirs)
    },
    selectAllDirs(dirs: LibraryDir[]) {
      this.selectedDirPaths = dirs.map((dir) => dir.rel_path)
    },
    clearSelection() {
      this.selectedDirPaths = []
    },
    toggleVeil() {
      this.veil = !this.veil
      try {
        localStorage.setItem(VEIL_KEY, this.veil ? '1' : '0')
      } catch {
        // Private mode: the option still holds for this session.
      }
    },
    setOverviewScroll(offset: number) {
      this.overviewScroll = offset
    },
  },
})
