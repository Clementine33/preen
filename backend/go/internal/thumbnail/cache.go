package thumbnail

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// diskCache is the local LRU of ADR 0013: one file per thumbnail, access
// time recorded as the file's mtime (atime is unreliable under relatime and
// noatime), oldest evicted once the total passes the budget. Nothing else
// knows about it, so deleting the directory at any time is safe.
type diskCache struct {
	dir      string
	capBytes int64

	mu sync.Mutex
	// size is the running total of the directory's bytes, -1 until first
	// measured.
	size int64
}

// cached content types, by the file extension the entry is stored under.
var cachedTypes = map[string]string{".jpg": "image/jpeg", ".png": "image/png"}

func (c *diskCache) read(key string) (Image, bool) {
	for ext, contentType := range cachedTypes {
		path := filepath.Join(c.dir, key+ext)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		now := time.Now()
		_ = os.Chtimes(path, now, now)
		return Image{Bytes: data, ContentType: contentType}, true
	}
	return Image{}, false
}

// write stores one thumbnail through a temporary file and a rename, so a
// reader never sees half of one.
func (c *diskCache) write(key string, img Image) {
	ext := ".jpg"
	if img.ContentType == "image/png" {
		ext = ".png"
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, "tmp-*")
	if err != nil {
		return
	}
	_, writeErr := tmp.Write(img.Bytes)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil || os.Rename(tmp.Name(), filepath.Join(c.dir, key+ext)) != nil {
		_ = os.Remove(tmp.Name())
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.size < 0 {
		c.size = c.measure(nil)
	} else {
		c.size += int64(len(img.Bytes))
	}
	if c.size > c.capBytes {
		c.evict()
	}
}

type cachedFile struct {
	path  string
	size  int64
	mtime time.Time
}

// measure totals the cached files, collecting them when files is non-nil.
func (c *diskCache) measure(files *[]cachedFile) int64 {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "tmp-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		if files != nil {
			*files = append(*files, cachedFile{filepath.Join(c.dir, entry.Name()), info.Size(), info.ModTime()})
		}
	}
	return total
}

// evict removes the least recently used files until the total is back under
// nine tenths of the budget, so the next few writes do not each pay a scan.
func (c *diskCache) evict() {
	var files []cachedFile
	c.size = c.measure(&files)
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })
	target := c.capBytes / 10 * 9
	for _, f := range files {
		if c.size <= target {
			return
		}
		if os.Remove(f.path) == nil {
			c.size -= f.size
		}
	}
}
