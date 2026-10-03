// Package execute runs frozen conversion components: prechecking inputs,
// materializing and validating outputs, committing replacements, then removing
// obsolete files. Its results retain partial outcomes on failure or cancellation.
//
// Encoder and RecoverFile supply media tools and soft deletion. The component
// pipeline also performs filesystem checks and commits directly. Callers own
// cross-run serialization, session scheduling and durable result persistence;
// execution does not reconcile inputs against live draft policy.
package execute
