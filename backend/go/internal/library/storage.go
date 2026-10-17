package library

import "errors"

// StorageClass describes the media storage, independently of the backend disk.
type StorageClass string

const (
	StorageHDD StorageClass = "hdd"
	StorageSSD StorageClass = "ssd"
)

var ErrInvalidStorageClass = errors.New("storage class must be hdd or ssd")

// MediaWorkers reserves one third of logical CPUs and bounds storage pressure.
func (c StorageClass) MediaWorkers(cpus int) int {
	budget := max(1, cpus*2/3)
	if c == StorageSSD {
		return min(16, budget)
	}
	return min(8, budget)
}

// EncodeWorkers includes encoding and output validation in the same budget.
func (c StorageClass) EncodeWorkers(cpus int) int {
	budget := max(1, cpus*2/3)
	if c == StorageSSD {
		return budget
	}
	return min(4, budget)
}
