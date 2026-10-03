// Package inventory owns the observed filesystem entries and their media
// facts: browsing, full-library scans, member refreshes and background media
// enrichment. Its scan pipeline stages complete observations before merging
// them, and reports progress and committed fact changes to callers.
//
// Traversal, probing and persistence are supplied through consumer seams.
// Library configuration, logical work metadata and desired conversion outputs
// belong to their respective consumers rather than to the observation model.
package inventory
