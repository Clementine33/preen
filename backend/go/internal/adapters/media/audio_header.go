package media

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
)

// Native header parsing for the formats a voice library is mostly made of:
// WAV, MP3 and AAC in MP4 (m4a). Each reads a few KiB of header where ffprobe
// reads up to megabytes to analyze the stream, which on a network share or a
// mechanical disk is most of the probe's cost (ADR 0013). The answers are
// ffprobe's own, computed the way ffmpeg computes them; anything these parsers
// do not recognize returns errNotNative and goes to ffprobe. They never guess.

var errNotNative = errors.New("not natively parsable")

// maxPlausibleSec rejects a header that decodes to nonsense (a week).
const maxPlausibleSec = 7 * 24 * 3600

// nativeAudio answers duration and, for the encoded formats, bitrate (bits per
// second; 0 when the header cannot give ffprobe's figure) from the header of a
// .wav, .mp3 or MP4-family file, errNotNative otherwise.
func nativeAudio(pathPosix string) (float64, int64, error) {
	ext := strings.ToLower(filepath.Ext(pathPosix))
	switch ext {
	case ".wav", ".mp3", ".m4a", ".m4b", ".mp4":
	default:
		return 0, 0, errNotNative
	}
	f, err := os.Open(filepath.FromSlash(pathPosix))
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	var sec float64
	var bitrate int64
	switch ext {
	case ".wav":
		sec, err = wavDuration(f)
	case ".mp3":
		sec, bitrate, err = mp3Facts(f, info.Size())
	default:
		sec, bitrate, err = mp4Facts(f, info.Size())
	}
	if err != nil {
		return 0, 0, err
	}
	if !(sec > 0) || math.IsInf(sec, 0) || sec > maxPlausibleSec {
		return 0, 0, errNotNative
	}
	return sec, bitrate, nil
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

// mp3Facts skips an ID3v2 tag and reads the first frame.
//
//   - A Xing or VBRI header gives the frame count, and with its byte count
//     the average bitrate: ffmpeg's av_rescale(bytes, 8*rate, frames*spf).
//   - LAME's Info header marks constant bitrate: the frame header's rate, and
//     its frame count for the length.
//   - No header: constant bitrate, the audio bytes over the frame header's
//     rate - ffprobe's estimate too. The frames that follow must all carry the
//     same rate; a stream that changes rate without a header is VBR whose
//     figure ffprobe takes from a decoded frame, so it goes to ffprobe.
func mp3Facts(r io.ReadSeeker, size int64) (float64, int64, error) {
	var tag [10]byte
	if _, err := io.ReadFull(r, tag[:]); err != nil {
		return 0, 0, errNotNative
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
		return 0, 0, errNotNative
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
		vbr := readVBRHeader(b[i:], h)
		switch {
		case vbr.frames == 0 && !constantRate(b[i:], h):
			return 0, 0, errNotNative
		case vbr.frames == 0:
			audio := size - start - int64(i)
			return float64(audio) * 8 / float64(h.bps()), h.bps(), nil
		}
		sec := float64(vbr.frames) * float64(h.samples) / float64(h.rate)
		if vbr.cbr {
			return sec, h.bps(), nil
		}
		var bitrate int64
		if vbr.bytes > 0 {
			den := int64(vbr.frames) * int64(h.samples)
			bitrate = (int64(vbr.bytes)*8*int64(h.rate) + den/2) / den
		}
		return sec, bitrate, nil
	}
	return 0, 0, errNotNative
}

type mp3Header struct {
	kbps, rate, samples, frameLen, sideInfo int
}

func (h mp3Header) bps() int64 { return int64(h.kbps) * 1000 }

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

// constantRate walks the frames that follow the first one through the buffer
// and reports whether every one carries its bitrate.
func constantRate(b []byte, first mp3Header) bool {
	for i, walked := first.frameLen, 0; walked < 32; walked++ {
		h, ok := parseMP3Header(b[min(i, len(b)):])
		if !ok {
			return true // the buffer ends, or tags/padding follow: nothing contradicts it
		}
		if h.kbps != first.kbps {
			return false
		}
		i += h.frameLen
	}
	return true
}

type vbrHeader struct {
	frames, bytes uint32
	cbr           bool // LAME's Info: a constant-bitrate file carrying a frame count
}

// readVBRHeader reads a Xing/Info header (after the side info) or a VBRI header
// (32 bytes into the frame). A zero frame count means none was found.
func readVBRHeader(frame []byte, h mp3Header) vbrHeader {
	if x := 4 + h.sideInfo; x+8 <= len(frame) {
		id := string(frame[x : x+4])
		if id == "Xing" || id == "Info" {
			flags := binary.BigEndian.Uint32(frame[x+4 : x+8])
			v := vbrHeader{cbr: id == "Info"}
			p := x + 8
			if flags&1 != 0 && p+4 <= len(frame) {
				v.frames = binary.BigEndian.Uint32(frame[p : p+4])
				p += 4
			}
			if flags&2 != 0 && p+4 <= len(frame) {
				v.bytes = binary.BigEndian.Uint32(frame[p : p+4])
			}
			return v
		}
	}
	if v := 4 + 32; v+18 <= len(frame) && string(frame[v:v+4]) == "VBRI" {
		return vbrHeader{
			bytes:  binary.BigEndian.Uint32(frame[v+10 : v+14]),
			frames: binary.BigEndian.Uint32(frame[v+14 : v+18]),
		}
	}
	return vbrHeader{}
}

// mp4Facts reads the box headers of an MP4 file.
//
//   - Duration: mvhd's duration over its timescale - the edit-trimmed length
//     ffprobe reports. The track's mdhd still counts the encoder priming
//     (ffmpeg's 1024 samples) and qaac writes no edit list.
//   - Bitrate: ffmpeg's mov figure, the sample bytes times 8 over the track
//     duration, truncated. With exactly one track and one mdat, the mdat
//     payload is the sample bytes; anything else answers 0 and goes to ffprobe.
//
// moov is found by box headers alone wherever it sits; the sample tables are
// never read. A fragmented file's mvhd carries no duration and goes to ffprobe.
func mp4Facts(r io.ReaderAt, size int64) (float64, int64, error) {
	top, err := mp4Boxes(r, 0, size)
	if err != nil {
		return 0, 0, err
	}
	var moov mp4Box
	var mdatBytes int64
	mdats := 0
	for _, b := range top {
		switch b.typ {
		case "moov":
			moov = b
		case "mdat":
			mdats++
			mdatBytes += b.end - b.body
		}
	}
	if moov.typ == "" {
		return 0, 0, errNotNative
	}
	children, err := mp4Boxes(r, moov.body, moov.end)
	if err != nil {
		return 0, 0, err
	}
	mvhd, ok := findMP4Box(children, "mvhd")
	if !ok {
		return 0, 0, errNotNative
	}
	timescale, duration, err := mp4Times(r, mvhd)
	if err != nil {
		return 0, 0, err
	}
	sec := float64(duration) / float64(timescale)

	var traks []mp4Box
	for _, b := range children {
		if b.typ == "trak" {
			traks = append(traks, b)
		}
	}
	if len(traks) != 1 || mdats != 1 {
		return sec, 0, nil
	}
	return sec, trackBitrate(r, traks[0], mdatBytes), nil
}

// trackBitrate is ffmpeg's data_size * 8 * time_scale / duration over the
// track's mdhd; 0 when the track cannot be read that far.
func trackBitrate(r io.ReaderAt, trak mp4Box, sampleBytes int64) int64 {
	mdhd, ok := mp4Path(r, trak, "mdia", "mdhd")
	if !ok || sampleBytes <= 0 || sampleBytes > math.MaxInt64/8 {
		return 0
	}
	timescale, duration, err := mp4Times(r, mdhd)
	if err != nil {
		return 0
	}
	hi, lo := bits.Mul64(uint64(sampleBytes)*8, uint64(timescale))
	if hi >= duration {
		return 0
	}
	q, _ := bits.Div64(hi, lo, duration)
	if q > math.MaxInt64 {
		return 0
	}
	return int64(q)
}

// mp4Times reads a movie or media header (mvhd and mdhd share the layout up to
// the duration): version 1 has 64-bit times. An all-ones duration means
// unknown.
func mp4Times(r io.ReaderAt, b mp4Box) (uint32, uint64, error) {
	if b.end-b.body < 32 {
		return 0, 0, errNotNative
	}
	var h [32]byte
	if _, err := r.ReadAt(h[:], b.body); err != nil {
		return 0, 0, errNotNative
	}
	var timescale uint32
	var duration uint64
	if h[0] == 1 {
		timescale, duration = binary.BigEndian.Uint32(h[20:24]), binary.BigEndian.Uint64(h[24:32])
		if duration == math.MaxUint64 {
			return 0, 0, errNotNative
		}
	} else {
		timescale, duration = binary.BigEndian.Uint32(h[12:16]), uint64(binary.BigEndian.Uint32(h[16:20]))
		if duration == math.MaxUint32 {
			return 0, 0, errNotNative
		}
	}
	if timescale == 0 || duration == 0 {
		return 0, 0, errNotNative
	}
	return timescale, duration, nil
}

type mp4Box struct {
	typ       string
	body, end int64
}

// mp4Boxes lists the boxes in [off, end), reading only their headers: a 64-bit
// size follows a size of 1, and a size of 0 runs to the end.
func mp4Boxes(r io.ReaderAt, off, end int64) ([]mp4Box, error) {
	var out []mp4Box
	for off+8 <= end {
		if len(out) == 256 { // a real file has a handful of boxes at each level
			return nil, errNotNative
		}
		var h [16]byte
		if _, err := r.ReadAt(h[:8], off); err != nil {
			return nil, errNotNative
		}
		size, headerLen := int64(binary.BigEndian.Uint32(h[0:4])), int64(8)
		switch size {
		case 1:
			if _, err := r.ReadAt(h[8:16], off+8); err != nil {
				return nil, errNotNative
			}
			large := binary.BigEndian.Uint64(h[8:16])
			if large > math.MaxInt64 {
				return nil, errNotNative
			}
			size, headerLen = int64(large), 16
		case 0:
			size = end - off
		}
		if size < headerLen || size > end-off {
			return nil, errNotNative
		}
		out = append(out, mp4Box{typ: string(h[4:8]), body: off + headerLen, end: off + size})
		off += size
	}
	return out, nil
}

func findMP4Box(boxes []mp4Box, typ string) (mp4Box, bool) {
	for _, b := range boxes {
		if b.typ == typ {
			return b, true
		}
	}
	return mp4Box{}, false
}

// mp4Path descends from a box through the named children.
func mp4Path(r io.ReaderAt, from mp4Box, path ...string) (mp4Box, bool) {
	cur := from
	for _, typ := range path {
		children, err := mp4Boxes(r, cur.body, cur.end)
		if err != nil {
			return mp4Box{}, false
		}
		var ok bool
		if cur, ok = findMP4Box(children, typ); !ok {
			return mp4Box{}, false
		}
	}
	return cur, true
}
