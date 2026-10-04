package filesystem_test

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
)

func TestNormalCoverPublicationDoesNotReopenPublishedImage(t *testing.T) {
	target, data := coverFixture(t)
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if _, err = unix.InotifyAddWatch(fd, filepath.Join(target.LibraryRoot, target.RelPath), unix.IN_OPEN); err != nil {
		t.Fatal(err)
	}
	files := filesystem.CoverFiles{}
	const suppliedDigest = "downloaded-byte-digest"
	result, err := files.Publish(t.Context(), target, data, suppliedDigest, true)
	if err != nil || result.Digest != suppliedDigest || result.File == nil || result.File.Size != int64(len(data)) {
		t.Fatalf("publication facts: %+v %v", result, err)
	}
	if err = files.CompletePublication(target); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 16384)
	n, err := unix.Read(fd, buffer)
	if err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < n; {
		length := int(binary.NativeEndian.Uint32(buffer[offset+12 : offset+16]))
		name := bytes.TrimRight(buffer[offset+unix.SizeofInotifyEvent:offset+unix.SizeofInotifyEvent+length], "\x00")
		if string(name) == "cover.png" {
			t.Fatal("normal publication reopened the new image")
		}
		offset += unix.SizeofInotifyEvent + length
	}
}
