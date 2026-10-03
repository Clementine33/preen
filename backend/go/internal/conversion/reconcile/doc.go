// Package reconcile decides the conversion needed to satisfy desired audio
// outputs from supplied inventory facts. It owns classification, grouping,
// policy comparison, blocking reasons and deterministic plan fingerprints.
//
// Reconciliation performs no filesystem, database or process I/O. Its outcomes
// describe a plan; callers acquire the facts, freeze the result and execute it.
package reconcile
