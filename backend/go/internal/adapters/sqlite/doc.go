// Package sqlite implements the backend's persistence and scan-staging seams.
// It owns SQL projections, nullable-value mapping, connection configuration and
// atomic operations spanning related records, including scan merges, ordered
// collection membership and the catalog's Favorites projection.
//
// Business packages own the types and consumer ports; this adapter persists
// them and enforces their transitions within transactions. It also implements
// development-schema initialization, release migrations, backups and database
// maintenance, rather than coordinating HTTP requests or task execution.
package sqlite
