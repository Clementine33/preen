// Package dirrename owns naming drafts, title filters, conflict detection and
// frozen directory rename execution. Planning reads selected roots' parents,
// never member inventories. Execution uses no-replace moves and atomic path
// relocation; startup repairs the first missing result without resuming a run.
// The stored and HTTP operation kind remains "rename".
package dirrename
