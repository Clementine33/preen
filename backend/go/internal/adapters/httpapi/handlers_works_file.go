package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/works"
)

// getWorkContentFile serves one manifest file's bytes (ADR 0013): the
// catalog has already answered the copy binding, the containment and the
// inventory whitelist; what is left here is the HTTP shape — the content
// type from the bytes, the charset a text body declares, and the range and
// revalidation semantics.
func (s *Server) getWorkContentFile(w http.ResponseWriter, r *http.Request) {
	service, ok := s.catalog(w)
	if !ok {
		return
	}
	code, ok := s.workPath(w, r)
	if !ok {
		return
	}
	loc, refusal := service.ContentFile(code, r.PathValue("copy"), r.PathValue("path"))
	if refusal != nil {
		writeWorksError(w, refusal)
		return
	}

	file, err := os.Open(loc.AbsPath)
	if err != nil {
		writeInternal(w, err, "the file could not be opened")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "WORK_NOT_FOUND", "the path is not a regular file")
		return
	}

	// The ETag keys on the manifest facts: size, mtime and the inventory's
	// content revision — a same-size rewrite bumps the revision, so a stale
	// cache never survives a rescan.
	etag := contentETag(loc)
	w.Header().Set("ETag", etag)
	// The address is not version-keyed, so a cached copy is revalidated
	// against the revision before reuse; a match answers 304 for the price of
	// a header round-trip.
	w.Header().Set("Cache-Control", "no-cache")
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if err := setWorkFileContentType(w.Header(), file, loc.Kind); err != nil {
		writeInternal(w, err, "the file could not be read")
		return
	}

	// ServeContent answers single-range 206 and sets Accept-Ranges. A
	// multi-range request is answered with the whole file instead: HTTP
	// allows ignoring a Range header, no caller of ours sends multi-range,
	// and partial-content sets serve nothing the page needs (ADR 0013).
	if strings.Contains(r.Header.Get("Range"), ",") {
		r.Header.Del("Range")
	}
	http.ServeContent(w, r, "", time.Unix(loc.Mtime, 0), file)
}

// contentETag keys one manifest file's identity on the facts the inventory
// rewrites when the bytes change.
func contentETag(loc works.ContentFileLocation) string {
	key := strconv.Itoa(loc.ContentRev) + ":" + strconv.FormatInt(loc.Mtime, 10) + ":" + strconv.FormatInt(loc.Size, 10)
	sum := sha256.Sum256([]byte(key))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// setWorkFileContentType names the bytes' own type: images, audio and video are
// sniffed from the content (the extension is not trusted), text declares the
// character set the body is actually in, and anything else is a stream.
func setWorkFileContentType(h http.Header, file *os.File, kind works.ContentFileKind) error {
	switch kind {
	case works.KindImage, works.KindAudio:
		sample, _, err := readSample(file, 512)
		if err != nil {
			return err
		}
		contentType := http.DetectContentType(sample)
		if kind == works.KindAudio && contentType == "audio/mpeg" {
			flac, err := flacBehindID3(file, sample)
			if err != nil {
				return err
			}
			if flac {
				contentType = "audio/flac"
			}
		}
		if kind == works.KindImage && !strings.HasPrefix(contentType, "image/") {
			contentType = "application/octet-stream"
		} else if kind == works.KindAudio && !strings.HasPrefix(contentType, "audio/") {
			// Go's sniffer misses FLAC, ADTS AAC and m4a (sniffed as video); name them from the
			// same table the scan uses, never the host MIME database.
			contentType = inventory.DetectFormatFromPath(file.Name(), false)
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}
		h.Set("Content-Type", contentType)
	case works.KindVideo:
		sample, _, err := readSample(file, 512)
		if err != nil {
			return err
		}
		contentType := http.DetectContentType(sample)
		if !strings.HasPrefix(contentType, "video/") {
			contentType = "video/mp4"
		}
		h.Set("Content-Type", contentType)
	case works.KindText, works.KindLyrics:
		sample, eof, err := readSample(file, 64*1024)
		if err != nil {
			return err
		}
		mediaType := "text/plain"
		if strings.EqualFold(filepath.Ext(file.Name()), ".vtt") {
			mediaType = "text/vtt"
		}
		h.Set("Content-Type", mediaType+"; charset="+sniffTextCharset(sample, eof))
	case works.KindUnknown:
		h.Set("Content-Type", "application/octet-stream")
	}
	return nil
}

// readSample reads the first n bytes of the file and rewinds it, so the read
// that types the content never steals from the read that serves it.
func readSample(file *os.File, n int) ([]byte, bool, error) {
	sample := make([]byte, n+1)
	read, err := io.ReadFull(file, sample)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, false, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, false, err
	}
	return sample[:min(read, n)], read <= n, nil
}

// flacBehindID3 reports whether an ID3v2 tag, which Go's sniffer takes for
// MP3, wraps a FLAC stream. The tag's syncsafe size says where the stream
// starts, which may lie beyond the sample; ReadAt leaves the offset alone.
func flacBehindID3(file *os.File, sample []byte) (bool, error) {
	if len(sample) < 10 || !bytes.HasPrefix(sample, []byte("ID3")) {
		return false, nil
	}
	offset := 10 + (int64(sample[6]&0x7f)<<21 | int64(sample[7]&0x7f)<<14 | int64(sample[8]&0x7f)<<7 | int64(sample[9]&0x7f))
	if sample[5]&0x10 != 0 { // a footer follows the tag
		offset += 10
	}
	magic := make([]byte, 4)
	if _, err := file.ReadAt(magic, offset); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	return string(magic) == "fLaC", nil
}

// sniffTextCharset names the character set a text body is in, by decoding a
// sample with each candidate in order and keeping the first that reads
// cleanly (ADR 0013: the bytes are served exactly as stored — the header
// tells the client how to decode, and a wrong guess is mojibake, which is
// why the sniff happens here, once, instead of in every client).
func sniffTextCharset(sample []byte, eof bool) string {
	if utf8.Valid(sample) {
		return "utf-8"
	}
	candidates := []struct {
		name    string
		decoder transform.Transformer
	}{
		{"utf-8", unicode.UTF8.NewDecoder()},
		{"shift_jis", japanese.ShiftJIS.NewDecoder()},
		{"gb18030", simplifiedchinese.GB18030.NewDecoder()},
	}
	for _, candidate := range candidates {
		decoded := make([]byte, len(sample)*4+4)
		n, _, err := candidate.decoder.Transform(decoded, sample, eof)
		// A bounded sample may stop in a valid multi-byte character. Only at
		// the real EOF does an incomplete suffix count as invalid encoding.
		if (err == nil || (!eof && errors.Is(err, transform.ErrShortSrc))) && !bytes.ContainsRune(decoded[:n], '�') {
			return candidate.name
		}
	}
	// Ambiguous bytes decode with replacements in every candidate: declare
	// the first legacy one rather than inventing an answer.
	return "shift_jis"
}
