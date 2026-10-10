// Package dirrename owns naming drafts (template, title filters, literal
// per-folder names), conflict detection, following folders renamed by hand,
// and frozen directory rename execution. A blocked folder is skipped, never a
// reason to refuse the run. Planning reads selected roots' parents,
// never member inventories. Execution uses no-replace moves and atomic path
// relocation; startup repairs the first missing result without resuming a run.
// The stored and HTTP operation kind remains "rename".
package dirrename
