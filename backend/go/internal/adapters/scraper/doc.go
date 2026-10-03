// Package scraper implements source clients for DLsite's independent static
// and dynamic channels, ASMR backup metadata and main-cover URL discovery. It
// owns HTTP requests, source decoding, retry/limit handling and translation into
// the snapshots accepted by the metadata consumer.
//
// Consumers choose which sources to refresh, import the snapshots and schedule
// cover delivery. This adapter does not own catalog persistence or batch sessions.
package scraper
