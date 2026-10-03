// Package httpapi exposes business entries through the loopback HTTP API. It
// owns routing, authentication, request decoding, wire projections, error/status
// mapping, SSE framing and HTTP semantics for served media bytes.
//
// Dependencies supplies business services rather than a database repository.
// Those entries own use-case policy and persistence; handlers translate the
// transport contract into calls and map their results back to the client.
package httpapi
