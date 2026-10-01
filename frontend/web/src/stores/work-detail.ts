import { defineStore } from 'pinia'
import {
  deriveTracks,
  dirAt,
  imageSets,
  lrcFilesOf,
  trackFormats,
  type ImageSet,
  type Track,
} from '@/features/work-detail/derive'
import type { ContentFileRef, WorkContent, WorkContentDir, WorkContentFile } from '@/lib/api/types'
import { usePlayerStore } from './player'

/**
 * The detail page's session state: the v3 prototype's `S`,
 * reactive and shared across the deck, leaf, bay and viewer. One work is one
 * session — binding a different rj resets everything personal to the visit
 * (scriptSeen, pairings, veil). Playback itself lives in the player store:
 * this store picks the file (`playFmt`, the format fallback) and hands the
 * player its address, which outlives the page.
 *
 * The content manifest is hydrated here by the page's query, because every
 * orchestration below — start a track, open a file, pair lyrics — reads it.
 * Catalog fields (nsfw, titles) are not duplicated: components read them
 * from the work detail query, which the mutation layer keeps fresh.
 */

/** Resolves a manifest file to its playable address; set by the page at bind. */
type FileUrl = (ref: ContentFileRef) => string
// A function, not state: it never needs to be reactive or reset with the visit.
let fileUrl: FileUrl | null = null

export type BayDock = 'bottom' | 'right'
export type BayTab = 'tracks' | 'files'
export type LeafMode = 'colophon' | 'lrc' | 'script'

export const useWorkDetailStore = defineStore('work-detail', {
  state: () => ({
    /** The bound work; null before the first bind. */
    rj: null as string | null,
    content: null as WorkContent | null,
    dock: 'bottom' as BayDock,
    bayOpen: false,
    tab: 'tracks' as BayTab,
    /** The browsed audio format (the bay's chips); position is never kept across a switch. */
    fmt: '',
    set: 0,
    idx: 0,
    flipped: false,
    viewerOpen: false,
    leaf: 'colophon' as LeafMode,
    /** Sticky once READ has opened: the switcher keeps the item afterwards. */
    scriptSeen: false,
    activeTxt: null as string | null,
    /** Track index → manually paired timed lyrics, this session only. */
    pairs: {} as Record<number, string>,
    /** The bay's LRC chip: the track list shows lyric files instead of audio (fmt stays as is). */
    lrcMode: false,
    /** The track the page is on (playing or last played), like v3's S.track. */
    track: 0,
    /** The format of the file actually playing. */
    playFmt: null as string | null,
    /** The R18 viewer option, off by default; hatches every set but the cover's. */
    veil: false,
    unveiled: false,
    /** The file browser's directory, as name segments from the copy root. */
    cwd: [] as string[],
    sel: null as string | null,
  }),

  getters: {
    /** The page reads the first online copy; offline ones stay listed. */
    activeCopy(state) {
      const online = state.content?.copies.find((copy) => copy.online)
      return online ?? state.content?.copies[0] ?? null
    },
    /** The manifest tree of the copy the page reads. */
    copy(): WorkContentDir | null {
      return this.activeCopy?.tree ?? null
    },
    sets(): ImageSet[] {
      return this.copy ? imageSets(this.copy) : []
    },
    /** Every timed-lyrics file of the copy, the LRC chip's rows. */
    lrcFiles(): WorkContentFile[] {
      return this.copy ? lrcFilesOf(this.copy) : []
    },
    tracks(): Track[] {
      return this.copy ? deriveTracks(this.copy) : []
    },
    /** The format chips the work's audio actually ships. */
    formats(): string[] {
      return trackFormats(this.tracks)
    },
    currentSet(state): ImageSet | null {
      return this.sets[state.set] ?? null
    },
    currentFile(state): WorkContentFile | null {
      return this.currentSet?.files[state.idx] ?? null
    },
    /** The automatic pairing: the same-basename timed lyrics of the given track. */
    autoLrc(): (track: number) => string | null {
      return (track) => this.tracks[track]?.lrcRel ?? null
    },
    /** The pairing that counts: manual first, then the automatic one. */
    lrcFor(state): (track: number) => string | null {
      return (track) => state.pairs[track] ?? this.tracks[track]?.lrcRel ?? null
    },
    /** The directory the file browser is in. */
    cwdDir(state): WorkContentDir | null {
      return this.copy ? dirAt(this.copy, state.cwd) : null
    },
  },

  actions: {
    /**
     * Hydrate the manifest of the bound work. A different rj is a new session:
     * everything resets. A refetch of the same rj keeps the session and only
     * clamps the selections against the fresh data.
     */
    bind(rj: string, content: WorkContent, resolveFileUrl?: FileUrl) {
      if (this.rj !== rj) this.$reset()
      // Without a resolver (a test binding the sample) tracks play silently.
      fileUrl = resolveFileUrl ?? null
      this.rj = rj
      this.content = content
      if (!this.formats.includes(this.fmt)) this.fmt = this.formats[0] ?? ''
      if (this.set >= this.sets.length) {
        this.set = 0
        this.idx = 0
        this.flipped = false
      } else if (this.idx >= this.sets[this.set].files.length) {
        this.idx = 0
        this.flipped = false
      }
      if (this.leaf === 'lrc' && !this.lrcFor(this.track)) this.leaf = 'colophon'
    },

    flip() {
      this.flipped = !this.flipped
    },

    /** Stepping always shows the front and clamps at the set's ends (v3's arrows). */
    step(delta: number) {
      const set = this.currentSet
      if (!set) return
      this.flipped = false
      const next = Math.min(Math.max(this.idx + delta, 0), set.files.length - 1)
      if (next !== this.idx) this.idx = next
    },

    showSet(set: number) {
      this.set = set
      this.idx = 0
      this.flipped = false
    },

    showImg(set: number, idx: number) {
      this.set = set
      this.idx = idx
      this.flipped = false
    },

    setLeaf(mode: LeafMode) {
      if (mode === 'script' && this.activeTxt === null) return
      if (mode === 'lrc' && !this.lrcFor(this.track)) mode = 'colophon'
      if (mode === 'script') this.scriptSeen = true
      this.leaf = mode
    },

    setDock(dock: BayDock) {
      this.dock = dock
    },

    /** A bay tab click both selects and lifts; the open tab clicked again closes (v3). */
    openBayTab(tab: BayTab) {
      if (this.dock === 'bottom') this.bayOpen = !(this.bayOpen && this.tab === tab)
      this.tab = tab
    },

    closeBay() {
      this.bayOpen = false
    },

    setFmt(fmt: string) {
      this.fmt = fmt
      this.lrcMode = false
    },

    showLrc() {
      this.lrcMode = true
    },

    toggleVeil() {
      this.veil = !this.veil
      this.unveiled = false
    },

    unveil() {
      this.unveiled = true
    },

    openViewer() {
      this.viewerOpen = true
    },

    closeViewer() {
      this.viewerOpen = false
    },

    /** Pair one track with timed lyrics by hand; the leaf follows into LISTEN. */
    pairLrc(track: number, rel: string) {
      this.pairs[track] = rel
      this.leaf = 'lrc'
    },

    /**
     * Play a track: followTrack picks the leaf, the file plays in the browsed
     * format or, when the track misses it, in its first one (v3's onlyIn).
     */
    startTrack(track: number, fmt?: string) {
      const entry = this.tracks[track]
      if (!entry) return
      this.track = track
      this.playFmt = fmt ?? (entry.files[this.fmt] ? this.fmt : entry.formats[0]) ?? null
      // A script being read is never interrupted; lyrics swap; else the colophon.
      if (this.leaf !== 'script') this.leaf = this.lrcFor(track) ? 'lrc' : 'colophon'
      if (this.rj === null) return
      const file = this.playFmt ? entry.files[this.playFmt] : undefined
      const copy = this.activeCopy
      const source = file && copy && fileUrl
        ? {
            src: fileUrl({ rj: this.rj, copyId: copy.copy_id, relPath: file.rel_path }),
            durationSec: file.duration_sec,
            format: file.format,
          }
        : undefined
      usePlayerStore().play({ rj: this.rj, title: entry.stem }, track, source)
    },

    /** A directory row navigates the browser. */
    selectDir(cwd: string[]) {
      this.cwd = cwd
    },

    /**
     * A file row: images locate their set, lyrics and texts open in READ,
     * audio starts its track; a non-audio pick closes the bottom drawer.
     */
    selectFile(f: WorkContentFile) {
      this.sel = f.rel_path
      if (f.kind === 'img') {
        const setIdx = this.sets.findIndex((set) => set.files.some((img) => img.rel_path === f.rel_path))
        if (setIdx >= 0) this.showImg(setIdx, this.sets[setIdx].files.findIndex((img) => img.rel_path === f.rel_path))
      } else if (f.kind === 'txt' || f.kind === 'lrc') {
        this.activeTxt = f.rel_path
        this.scriptSeen = true
        this.leaf = 'script'
      } else if (f.kind === 'aud') {
        const track = this.tracks.findIndex((entry) => Object.values(entry.files).some((a) => a?.rel_path === f.rel_path))
        if (track >= 0) this.startTrack(track, f.format ?? undefined)
      }
      if (this.dock === 'bottom' && f.kind !== 'aud') this.bayOpen = false
    },
  },
})
