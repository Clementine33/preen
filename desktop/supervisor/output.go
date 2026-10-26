package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const tailBytes = 32 * 1024

type output struct {
	mu   sync.Mutex
	file *os.File
	tail []byte
	err  error
}

func openOutput(dataDir string) (*output, error) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	current, previous := filepath.Join(dir, "preen.log"), filepath.Join(dir, "preen.prev.log")
	if _, err := os.Stat(current); err == nil {
		if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := os.Rename(current, previous); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(current, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	return &output{file: f}, nil
}

func (o *output) Write(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tail = append(o.tail, b...)
	if len(o.tail) > tailBytes {
		o.tail = append([]byte(nil), o.tail[len(o.tail)-tailBytes:]...)
	}
	n, err := o.file.Write(b)
	o.err = errors.Join(o.err, err)
	return n, err
}

func (o *output) finish() (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(o.tail), errors.Join(o.err, o.file.Close())
}
