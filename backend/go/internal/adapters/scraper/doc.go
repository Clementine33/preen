// Package scraper implements source clients for DLsite's independent static
// and dynamic channels, ASMR backup metadata and main-cover URL discovery. It
// owns HTTP requests, source decoding, retry/limit handling and translation into
// the snapshots accepted by the metadata consumer.
//
// Requests leave through the Outbound Proxy when one is set. The proxy comes
// only from that user setting, resolved per request so a change applies to the
// next one: the process environment's HTTP(S)_PROXY is ignored, so one visible
// setting decides how the server reaches the network, and a container's
// ambient variables cannot silently reroute it.
//
// Consumers choose which sources to refresh, import the snapshots and schedule
// cover delivery. This adapter does not own catalog persistence or batch sessions.
package scraper
