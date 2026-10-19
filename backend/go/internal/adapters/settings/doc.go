// Package settings delivers the embedded initial config.json on first startup
// and reads configuration through the server and conversion consumers' seams.
// It owns the file's wire shape, defaults and normalization. Initial delivery
// never replaces existing files; tag readers return empty sets for missing or
// broken files, without falling back to the embedded initial configuration.
//
// Consumers apply these settings to business decisions and scheduling. This
// reader does not own workset drafts or persist their operation-specific policy.
package settings
