// Package scraper implements source clients for DLsite's independent static
// and dynamic channels, ASMR backup metadata and main-cover URL discovery. It
// owns HTTP requests, source decoding, retry/limit handling and translation into
// the snapshots accepted by the metadata consumer.
//
// Requests leave through the Outbound Proxy, resolved per request so a change
// applies to the next one. One visible setting decides how the server reaches
// the network: the operating system's proxy (including HTTP(S)_PROXY) is
// consulted only while that setting says to follow the system.
//
// Consumers choose which sources to refresh, import the snapshots and schedule
// cover delivery. This adapter does not own catalog persistence or batch sessions.
package scraper
