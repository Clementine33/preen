// Package thumbnail serves image thumbnails as a disposable derived cache
// (ADR 0015): requests for one address share a computation, computations are
// bounded by an admission queue, and results land in an on-disk LRU that may
// be deleted at any time. The scaling itself is the Engine port.
package thumbnail

import (
	"errors"
	"sync/atomic"

	"golang.org/x/sync/singleflight"
)

// MaxPixels is the decode guard (ADR 0015 §3): a source larger than this is
// refused before its pixels are decoded, so a decompression bomb costs only a
// header read.
const MaxPixels = 100_000_000

var (
	// ErrBusy refuses a request when the admission queue is full; the caller
	// answers 503 with Retry-After.
	ErrBusy = errors.New("thumbnail: admission queue full")
	// ErrTooLarge refuses a source over MaxPixels.
	ErrTooLarge = errors.New("thumbnail: source exceeds the pixel limit")
	// ErrUndecodable refuses a source the engine cannot read as an image.
	ErrUndecodable = errors.New("thumbnail: source is not a decodable image")
)

// Image is one encoded thumbnail.
type Image struct {
	Bytes       []byte
	ContentType string
}

// Engine decodes one source file and scales it to the target height, never
// upscaling (ADR 0015 §5). It returns ErrTooLarge or ErrUndecodable for the
// refusals the caller reports.
type Engine interface {
	Thumbnail(path string, height int) (Image, error)
}

// Source is one image to thumbnail: the validated path, and the version its
// address carries (the cache is keyed on both).
type Source struct {
	Path    string
	Version string
}

// Service is the request path of ADR 0015 §3 below the HTTP layer.
type Service struct {
	engine  Engine
	cache   *diskCache
	flights singleflight.Group
	// slots bounds concurrent computations; waiting counts requests queued for
	// one, refused past maxWaiting.
	slots      chan struct{}
	waiting    atomic.Int32
	maxWaiting int32
}

// New builds the service: workers concurrent computations, at most queue
// requests waiting for one, and a cache under cacheDir of about capBytes.
func New(engine Engine, cacheDir string, capBytes int64, workers, queue int) *Service {
	return &Service{
		engine:     engine,
		cache:      &diskCache{dir: cacheDir, capBytes: capBytes, size: -1},
		slots:      make(chan struct{}, workers),
		maxWaiting: int32(queue), //nolint:gosec // a small configured queue length
	}
}

// heights are the served target heights. A request snaps up to the nearest,
// so arbitrary heights neither fragment the cache nor multiply the work.
var heights = []int{120, 240, 480, 960}

// SnapHeight maps a requested height to the served one; false for a height
// that is not positive.
func SnapHeight(h int) (int, bool) {
	if h <= 0 {
		return 0, false
	}
	for _, served := range heights {
		if h <= served {
			return served, true
		}
	}
	return heights[len(heights)-1], true
}
