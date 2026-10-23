package conversion

// Catalog owns the conversion classifier tag library and its validation.
// Configuration supplies read-only defaults; storage holds user-added tags.
type Catalog struct {
	tags     ClassifierTagStore
	settings Settings
}

// NewCatalog wires the tag library to its storage and configuration.
func NewCatalog(tags ClassifierTagStore, settings Settings) *Catalog {
	return &Catalog{tags: tags, settings: settings}
}
