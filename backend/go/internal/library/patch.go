package library

// Patch applies a partial edit of the name and storage class: only the fields
// the caller sent change, the rest keep the value the current row holds. The
// root is fixed at creation, so no path moves and no admission slot is needed.
func (s *Service) Patch(id string, name *string, storageClass *StorageClass) (*Library, error) {
	if storageClass != nil {
		if *storageClass != StorageHDD && *storageClass != StorageSSD {
			return nil, ErrInvalidStorageClass
		}
	}
	return s.store.UpdateLibrary(id, name, storageClass)
}
