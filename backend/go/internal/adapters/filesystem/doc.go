// Package filesystem implements traversal, directory inspection, checked moves,
// recovery and cover publication for the backend's consumer seams. It reports
// filesystem observations and confines writes to checked paths, including the
// platform-specific no-replace operation.
//
// Consumers select the business operation and coordinate its lifecycle,
// admission and inventory refresh. This adapter owns the disk mechanisms and
// publication recovery, not the workset or metadata-refresh lifecycle.
package filesystem
