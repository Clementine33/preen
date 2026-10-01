package media

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Native duration parsing for the two formats a voice library is mostly made
// of. Each reads a few KiB of header where ffprobe reads up to megabytes to
// analyze the stream, which on a network share or a mechanical disk is most of
// the probe's cost (ADR 0015 §2). Anything these parsers do not recognize
// returns errNotNative and goes to ffprobe; they never guess.

var errNotNative = errors.New("not natively parsable")

// maxPlausibleSec rejects a header that decodes to nonsense (a week).
const maxPlausibleSec = 7 * 24 * 3600

// nativeDuration answers from the header for .wav and .mp3, errNotNative
// otherwise.
func nativeDuration(pathPosix string) (float64, error) {
	ext := strings.ToLower(filepath.Ext(pathPosix))
	if ext != ".wav" && ext != ".mp3" {
		return 0, errNotNative
	}
	f, err := os.Open(filepath.FromSlash(pathPosix))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var sec float64
	if ext == ".wav" {
		sec, err = wavDuration(f)
	} else {
		var info os.FileInfo
		if info, err = f.Stat(); err == nil {
			sec, err = mp3Duration(f, info.Size())
		}
	}
	if err != nil {
		return 0, err
	}
	if !(sec > 0) || math.IsInf(sec, 0) || sec > maxPlausibleSec {
		return 0, errNotNative
	}
	return sec, nil
}

// wavDuration walks the RIFF chunks to the data chunk: its size over the fmt
// chunk's byte rate. RF64 and streamed WAVs (a data size of 0 or all ones)
// carry no usable size here and go to ffprobe.
func wavDuration(r io.ReadSeeker) (float64, error) {
	br := bufio.NewReaderSize(r, 4096)
	var head [12]byte
	if _, err := io.ReadFull(br, head[:]); err != nil {
		return 0, errNotNative
	}
	if string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return 0, errNotNative
	}
	var byteRate uint32
	pos := int64(12)
	for range 64 { // a real file has a handful of chunks before data
		var chunk [8]byte
		if _, err := io.ReadFull(br, chunk[:]); err != nil {
			return 0, errNotNative
		}
		pos += 8
		id, size := string(chunk[0:4]), binary.LittleEndian.Uint32(chunk[4:8])
		switch id {
		case "fmt ":
			// 16 (PCM) to 40 (extensible); never allocate what the file claims.
			if size < 16 || size > 256 {
				return 0, errNotNative
			}
			body := make([]byte, size)
			if _, err := io.ReadFull(br, body); err != nil {
				return 0, errNotNative
			}
			byteRate = binary.LittleEndian.Uint32(body[8:12])
			pos += int64(size) + int64(size&1)
			if size&1 == 1 {
				if _, err := br.Discard(1); err != nil {
					return 0, errNotNative
				}
			}
		case "data":
			if byteRate == 0 || size == 0 || size == math.MaxUint32 {
				return 0, errNotNative
			}
			return float64(size) / float64(byteRate), nil
		default:
			// Chunks are word-aligned: an odd size carries one pad byte.
			pos += int64(size) + int64(size&1)
			if _, err := r.Seek(pos, io.SeekStart); err != nil {
				return 0, errNotNative
			}
			br.Reset(r)
		}
	}
	return 0, errNotNative
}

var (
	// mp3Kbps is indexed [MPEG-1 or not][layer 1..3][bitrate index].
	mp3Kbps = [2][3][15]int{
		{
			{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448},
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384},
			{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
		},
		{
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
		},
	}
	// mp3Rates is indexed [version bits][sample-rate index]; version 1 is reserved.
	mp3Rates = [4][3]int{{11025, 12000, 8000}, {}, {22050, 24000, 16000}, {44100, 48000, 32000}}
)

// mp3Duration skips an ID3v2 tag and reads the first frame. A Xing/Info or
// VBRI header gives the exact frame count; without one the stream is constant
// bitrate and the audio bytes over the bitrate give the length — the same
// estimate ffprobe makes for such a file.
func mp3Duration(r io.ReadSeeker, size int64) (float64, error) {
	var tag [10]byte
	if _, err := io.ReadFull(r, tag[:]); err != nil {
		return 0, errNotNative
	}
	start := int64(0)
	if string(tag[0:3]) == "ID3" {
		// The size is synchsafe: four 7-bit groups.
		tagSize := int64(tag[6]&0x7f)<<21 | int64(tag[7]&0x7f)<<14 | int64(tag[8]&0x7f)<<7 | int64(tag[9]&0x7f)
		start = 10 + tagSize
		if tag[5]&0x10 != 0 { // footer present
			start += 10
		}
	}
	if _, err := r.Seek(start, io.SeekStart); err != nil {
		return 0, errNotNative
	}
	buf := make([]byte, 8192)
	n, _ := io.ReadFull(r, buf)
	b := buf[:n]
	for i := 0; i+4 <= len(b); i++ {
		h, ok := parseMP3Header(b[i:])
		if !ok {
			continue
		}
		// A false sync inside junk is common; a real frame is followed by another.
		if next := i + h.frameLen; next+4 <= len(b) {
			if _, ok := parseMP3Header(b[next:]); !ok {
				continue
			}
		}
		if frames, ok := vbrFrames(b[i:], h); ok {
			return float64(frames) * float64(h.samples) / float64(h.rate), nil
		}
		audio := size - start - int64(i)
		return float64(audio) * 8 / float64(h.kbps*1000), nil
	}
	return 0, errNotNative
}

type mp3Header struct {
	kbps, rate, samples, frameLen, sideInfo int
}

// parseMP3Header decodes a frame header, rejecting reserved and free-format
// values (free format has no bitrate to estimate from).
func parseMP3Header(b []byte) (mp3Header, bool) {
	if len(b) < 4 || b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return mp3Header{}, false
	}
	version, layerBits := (b[1]>>3)&3, (b[1]>>1)&3
	bitrateIdx, rateIdx := b[2]>>4, (b[2]>>2)&3
	if version == 1 || layerBits == 0 || bitrateIdx == 0 || bitrateIdx == 15 || rateIdx == 3 {
		return mp3Header{}, false
	}
	mpeg1 := version == 3
	layer := 4 - int(layerBits) // 1, 2 or 3
	vi := 1
	if mpeg1 {
		vi = 0
	}
	h := mp3Header{kbps: mp3Kbps[vi][layer-1][bitrateIdx], rate: mp3Rates[version][rateIdx]}
	pad := int(b[2]>>1) & 1
	switch {
	case layer == 1:
		h.samples = 384
		h.frameLen = (12*h.kbps*1000/h.rate + pad) * 4
	case layer == 3 && !mpeg1:
		h.samples = 576
		h.frameLen = 72*h.kbps*1000/h.rate + pad
	default:
		h.samples = 1152
		h.frameLen = 144*h.kbps*1000/h.rate + pad
	}
	mono := b[3]>>6 == 3
	switch {
	case mpeg1 && !mono:
		h.sideInfo = 32
	case mpeg1, !mono: // MPEG-1 mono, MPEG-2/2.5 stereo
		h.sideInfo = 17
	default: // MPEG-2/2.5 mono
		h.sideInfo = 9
	}
	return h, h.frameLen > 4
}

// vbrFrames reads the frame count of a Xing/Info header (after the side info)
// or a VBRI header (32 bytes into the frame).
func vbrFrames(frame []byte, h mp3Header) (uint32, bool) {
	if x := 4 + h.sideInfo; x+12 <= len(frame) {
		id := frame[x : x+4]
		if (bytes.Equal(id, []byte("Xing")) || bytes.Equal(id, []byte("Info"))) &&
			binary.BigEndian.Uint32(frame[x+4:x+8])&1 != 0 {
			if frames := binary.BigEndian.Uint32(frame[x+8 : x+12]); frames > 0 {
				return frames, true
			}
		}
	}
	if v := 4 + 32; v+18 <= len(frame) && bytes.Equal(frame[v:v+4], []byte("VBRI")) {
		if frames := binary.BigEndian.Uint32(frame[v+14 : v+18]); frames > 0 {
			return frames, true
		}
	}
	return 0, false
}
