// Package refresh owns source metadata refresh: group selection, source fallback,
// ancestor retrieval and imports through the work catalog's validation boundary.
// It depends on works; the base catalog does not depend on this workflow.
// Its only production workflow entry is a workset execution, so Task, revision
// policy and batch execution live here together. The stored operation kind is
// still "metadata_refresh"; package placement does not change payloads or events.
package refresh
