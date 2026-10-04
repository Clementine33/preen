// Package coverdelivery owns durable main-cover requests, shared downloads and
// per-copy delivery outcomes, including cancellation, retry and restart recovery.
// Its placement under works states ownership of work-oriented delivery state;
// it does not imply an import of the base works package. Source refresh queues
// delivery here without waiting for publication. Downloader, Files and Staging
// provide mechanisms; this package decides when bytes may be reused or published.
package coverdelivery
