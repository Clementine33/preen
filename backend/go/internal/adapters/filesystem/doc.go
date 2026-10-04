// Package filesystem implements traversal, directory inspection, checked moves,
// recovery, resolved containment and cover publication/staging for consumer
// seams. It confines writes to checked paths and owns platform-specific
// no-replace operations and UNC recovery-name byte limits.
//
// Consumers select the business operation and coordinate its lifecycle,
// admission and inventory refresh. This adapter owns the disk mechanisms and
// publication recovery, not the workset or metadata-refresh lifecycle.
package filesystem
