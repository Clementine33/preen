package library

// Create registers a library for a root path. The root's canonical identity is
// the duplicate boundary (ErrLibraryExists); strict containment is ErrLibraryRootOverlap.
// Storage validates containment and writes the row in one transaction.
func (s *Service) Create(name, rootPath string) (*Library, error) {
	return s.store.CreateLibrary(name, rootPath)
}
