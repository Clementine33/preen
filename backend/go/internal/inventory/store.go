package inventory

// pipelineRepo is the optional half of the staging store: a store that can
// write whole batches gets that path used.
type pipelineRepo interface {
	WriteStagingBatch(sessionID string, entries []StagingEntry) error
}
