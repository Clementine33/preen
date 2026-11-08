import type { LocationQuery } from 'vue-router'
import type { Library, LibraryDir } from '@/lib/api/types'
import { defineStore } from 'pinia'

/**
 * UI-only store: the workbench's active library, the overview's directory
 * selection and the work wall's remembered view. Server-derived data lives in
 * Vue Query; this store never holds libraries, directories, loading, or error
 * state. `activeLibraryId` belongs to the workbench: the library last entered
 * this session, null before any (it is never filled from the list). The wall's
 * library lives in its own address and reaches this store only through
 * `wallQuery`; `startLibraryId` is the one id mirrored here from the queries.
 *
 * The selection is a set of library-relative paths, because that is the
 * identity a record is created from — a folder id from an earlier scan would
 * have been renumbered by the next one.
 */
const VEIL_KEY = 'preen.r18-veil'

function readVeil(): boolean {
  try {
    return localStorage.getItem(VEIL_KEY) === '1'
  }
  catch {
    return false
  }
}

export const useLibraryUiStore = defineStore('library-ui', {
  state: () => ({
    activeLibraryId: null as string | null,
    /**
     * The library a session with no place of its own opens: the Default Library
     * while it exists, else the newest. Mirrored from the library list and the
     * preferences by `useLibraryList`; null until both have answered.
     */
    startLibraryId: null as string | null,
    /** The last `/works` query of this session that named a library; never in localStorage, no action resets it. */
    wallQuery: {} as LocationQuery,
    selectedDirPaths: [] as string[],
    /**
     * The overview list's scroll offset, kept across a trip into a member's
     * files and back: leaving the list must not lose the reader's place.
     */
    overviewScroll: 0,
    /** The R18 viewer option, off by default; kept in localStorage, set on the wall, read by the wall and the detail page. */
    veil: readVeil(),
  }),
  getters: {
    /** Membership view of the ordered `selectedDirPaths`. */
    selectedDirSet: state => new Set(state.selectedDirPaths),
    /**
     * The library a work's copy is preferred from: the wall's remembered one
     * (null under its all-libraries scope), else the start library.
     */
    scopedLibraryId: (state): string | null => {
      const library = state.wallQuery.library
      if (library === 'all')
        return null
      return typeof library === 'string' && library !== ''
        ? library
        : state.startLibraryId
    },
  },
  actions: {
    setActiveLibrary(id: string) {
      if (id === this.activeLibraryId)
        return
      this.activeLibraryId = id
      // Switching libraries clears the previous library's temporary
      // selection; a scroll offset of another list means nothing here either.
      this.selectedDirPaths = []
      this.overviewScroll = 0
    },
    // Keeps the active ID valid against a freshly fetched library list: it is
    // dropped when its library is gone, and never filled in.
    reconcileLibraries(libraries: Library[]) {
      if (this.activeLibraryId !== null && !libraries.some(library => library.id === this.activeLibraryId)) {
        this.activeLibraryId = null
        this.selectedDirPaths = []
      }
    },
    // Drops selected paths that no longer exist in a successful listing.
    // Listings from another library are ignored, so a late response or a
    // background refetch can never mutate the current selection.
    reconcileDirs(libraryId: string, dirs: LibraryDir[]) {
      if (libraryId !== this.activeLibraryId)
        return
      if (this.selectedDirPaths.length === 0)
        return
      const paths = new Set(dirs.map(dir => dir.rel_path))
      const next = this.selectedDirPaths.filter(path => paths.has(path))
      if (next.length !== this.selectedDirPaths.length)
        this.selectedDirPaths = next
    },
    // The optional `dirs` argument restores the existence guard: a click that
    // lands after reconcileDirs dropped a path (a scan-sync refresh removed it
    // right before the event) must not push a stale path into the selection.
    toggleDir(relPath: string, dirs?: LibraryDir[]) {
      if (this.selectedDirPaths.includes(relPath)) {
        this.selectedDirPaths = this.selectedDirPaths.filter(selected => selected !== relPath)
      }
      else {
        if (dirs && !dirs.some(dir => dir.rel_path === relPath))
          return
        this.selectedDirPaths.push(relPath)
      }
    },
    setDirSelected(relPath: string, selected: boolean, dirs?: LibraryDir[]) {
      if (selected !== this.selectedDirPaths.includes(relPath))
        this.toggleDir(relPath, dirs)
    },
    selectAllDirs(dirs: LibraryDir[]) {
      this.selectedDirPaths = dirs.map(dir => dir.rel_path)
    },
    // Adds the given dirs to the selection in one assignment; a selection the
    // list does not show (a search filter hides it) stays.
    addDirs(dirs: LibraryDir[]) {
      const have = this.selectedDirSet
      const added = dirs.map(dir => dir.rel_path).filter(path => !have.has(path))
      if (added.length > 0) {
        this.selectedDirPaths = [
          ...this.selectedDirPaths,
          ...added,
        ]
      }
    },
    clearSelection() {
      this.selectedDirPaths = []
    },
    toggleVeil() {
      this.veil = !this.veil
      try {
        localStorage.setItem(VEIL_KEY, this.veil ? '1' : '0')
      }
      catch {
        // Private mode: the option still holds for this session.
      }
    },
    setOverviewScroll(offset: number) {
      this.overviewScroll = offset
    },
  },
})
