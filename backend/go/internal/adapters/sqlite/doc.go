// Package sqlite implements the backend's persistence and scan-staging seams.
// It owns SQL projections, nullable-value mapping, connection configuration and
// atomic operations spanning related records, including scan merges, ordered
// collection membership and the catalog's Favorites projection. Scan sessions
// own dedicated physical connections: TEMP-only staging leaves the main writer
// free, and the same connection merges atomically before releasing its temp files.
//
// A positive inventory directory mtime is paired with its complete direct-entry
// listing, including the library root. Synthetic parents and execution ancestors
// use 0; scoped refreshes reset an unobserved scope root to 0. Directory relocation
// preserves mtimes.
// New directory writers without a complete paired observation must use 0.
//
// Business packages own the types and consumer ports; this adapter persists
// them and enforces their transitions within transactions. It also implements
// development-schema initialization, release migrations, backups and database
// maintenance. Retention purges terminal session ledgers and unreferenced done
// cover downloads in bounded batches; cover delivery reports and files remain
// owned by their respective records and filesystem adapters. Generated plans
// stage in guarded batches and publish atomically; invisible retired plans and
// staging left by terminal generations are deleted in single-table batches.
package sqlite
