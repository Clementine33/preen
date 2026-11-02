// Package updatecheck owns the process's one update check: whether a newer
// stable release than the running version has been published. It holds the
// result in memory, so there is nothing to persist or migrate, and exposes it
// as a Status that is checking until the single background check settles.
//
// The check runs once per process, from Run. Reading the release is a port
// (Source) implemented by an adapter; this package owns when to ask, the retry
// policy, the semantic-version comparison and the decision that a failure is
// reported like "up to date": the user is not told about a failed check, only
// the Diagnostic Log is.
package updatecheck
