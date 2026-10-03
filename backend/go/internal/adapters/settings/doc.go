// Package settings reads config.json and supplies live configuration through
// the conversion and scanning consumers' seams. It owns the file's wire shape,
// defaults and normalization, including tool paths and worker limits.
//
// Consumers apply these settings to business decisions and scheduling. This
// reader does not own workset drafts or persist their operation-specific policy.
package settings
