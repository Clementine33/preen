// Package githubrelease reads the latest stable release of Preen from the
// GitHub REST API for the update check. It owns the HTTP request, the decoding
// of the release and the classification of failures into those worth another
// attempt (network errors, 5xx, 429) and those that are not.
//
// Requests leave through the Outbound Proxy, resolved per request so a change
// applies to the next one (ADR 0019). An unreadable proxy setting fails the
// request; it never falls back to a direct connection. Deciding when to ask,
// retrying and comparing versions belong to the update check, not here.
package githubrelease
