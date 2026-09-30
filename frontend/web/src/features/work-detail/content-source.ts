// The work content seam (draft spec §1): the detail page's single doorway to
// file bytes and URLs. Components and queries import only this file, so
// swapping the mock for the API client (Phase C) is a change to this file
// alone — no component rewrite, no contract drift: the mock's types ARE the
// contract (lib/api/types.ts WorkContent*).
//
// Phase A: every function re-exports the mock. Phase C: loadWorkContent calls
// the manifest route (draft spec §4 B2), the URL builders compose it with the
// file-content route (B3) and the thumbnail route (B4), loadText fetches text
// and lets the response header's charset decode it.

export { loadWorkContent, imageUrl, thumbUrl, audioUrl, loadText } from './mock-content'

/** The identity of one file of one copy: what every seam function addresses. */
export interface ContentFileRef {
  rj: string
  copyId: string
  relPath: string
}
