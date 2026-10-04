// Package dirrename owns directory naming drafts, template validation, conflict
// detection and revision facts. Its workset Task plans names without writing
// directories; catalog and directory occupancy arrive through read-only ports.
// Planning reads only selected work roots' parents, never member inventories.
// The stored and HTTP operation kind remains "rename" regardless of package name.
package dirrename
