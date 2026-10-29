package works

import "context"

// LocalCover names the cover file a listed work already has on disk. It is a
// presentation projection: the catalog neither computes nor stores it, and the
// HTTP layer asks a LocalCovers reader for the page it is about to answer.
// The image bytes keep going through the content route.
type LocalCover struct {
	CopyID  string
	RelPath string
	// Version changes whenever the inventory observes new bytes at the path.
	Version string
	// Size, Mtime and ContentRev are the file facts its thumbnail version
	// keys on (Service.CoverThumbV).
	Size       int64
	Mtime      int64
	ContentRev int
}

// LocalCovers answers, per work, the winning copy's cover; works without one
// are absent from the map.
type LocalCovers interface {
	LocalCovers(ctx context.Context, ids []WorkCode) (map[WorkCode]LocalCover, error)
}
