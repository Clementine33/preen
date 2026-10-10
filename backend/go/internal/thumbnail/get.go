package thumbnail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
)

// Get answers one thumbnail: from the cache, or from a computation shared by
// every concurrent request for the same address (ADR 0013).
//
// A request that goes away leaves: while its computation is still queued the
// queued slot is withdrawn, and a computation already admitted runs to the
// cache instead of being aborted, so the other waiters and the next visit
// still get it.
func (s *Service) Get(ctx context.Context, src Source, height int) (Image, error) {
	key := cacheKey(src, height)
	if img, ok := s.cache.read(key); ok {
		return img, nil
	}
	for {
		flight := s.flights.DoChan(key, func() (any, error) {
			return s.compute(ctx, key, src.Path, height)
		})
		select {
		case <-ctx.Done():
			return Image{}, ctx.Err()
		case result := <-flight:
			// The flight's leader left the queue; this request is still here,
			// so it leads the next flight.
			if isCancel(result.Err) && ctx.Err() == nil {
				continue
			}
			if result.Err != nil {
				return Image{}, result.Err
			}
			img, _ := result.Val.(Image)
			return img, nil
		}
	}
}

// compute waits for an admission slot under the leader's context, then
// renders and caches detached from it.
func (s *Service) compute(ctx context.Context, key, path string, height int) (Image, error) {
	release, err := s.admit(ctx)
	if err != nil {
		return Image{}, err
	}
	defer release()
	img, err := s.engine.Thumbnail(path, height)
	if err != nil {
		return Image{}, err
	}
	// Best effort: a failed write only costs the next request a recomputation.
	s.cache.write(key, img)
	return img, nil
}

// admit takes a computation slot, queueing for one while the queue has room.
func (s *Service) admit(ctx context.Context) (func(), error) {
	release := func() { <-s.slots }
	select {
	case s.slots <- struct{}{}:
		return release, nil
	default:
	}
	if s.waiting.Add(1) > s.maxWaiting {
		s.waiting.Add(-1)
		return nil, ErrBusy
	}
	defer s.waiting.Add(-1)
	select {
	case s.slots <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func isCancel(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// cacheKey names one thumbnail: the source path (two files can share their
// facts), its version and the served height.
func cacheKey(src Source, height int) string {
	sum := sha256.Sum256([]byte(src.Path + "\x00" + src.Version + "\x00" + strconv.Itoa(height)))
	return hex.EncodeToString(sum[:])
}
