// Package coverdelivery owns durable main-cover requests and per-copy delivery
// outcomes, including cancellation, retry and restart recovery.
// Its placement under works states ownership of work-oriented delivery state;
// it does not imply an import of the base works package. Source refresh queues
// delivery here without waiting for publication. Downloader and Files provide
// mechanisms; this package decides when a download happens and what is published.
// A worker claims one pending delivery together with every other pending
// delivery of the same work, downloads the work's current cover URL once into
// memory (falling back to the ASMR.one backup) and publishes it to each copy.
// Restart returns claimed deliveries to pending and downloads again; bytes are
// never retained. Refresh always downloads and relies on the digest to skip
// identical files.
// Delivery uses frozen paths and synchronizes observed cover inventory deltas,
// retaining publication journals until inventory acknowledgment succeeds.
// After a metadata batch and its own delivery demands finish, it repairs only
// targets whose inventory synchronization failed, in one selected-range scan.
// Successful deliveries need no scan or file-operation admission. Follow-ups
// are process-local; explicit retries retain only exact inventory synchronization.
package coverdelivery
