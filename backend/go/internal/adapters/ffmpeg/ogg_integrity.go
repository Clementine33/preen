package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// hashOpus validates the single logical Ogg stream emitted by this adapter and
// hashes the original bytes in the same pass. Page CRC detects payload damage;
// sequence, continuation and EOS checks detect missing/truncated pages.
func hashOpus(ctx context.Context, file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	hash := sha256.New()
	var header [27]byte
	var lacing [255]byte
	payload := make([]byte, 255*255)
	var serial, sequence uint32
	first, eos, continued := true, false, false
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := io.ReadFull(file, header[:])
		if eos {
			if n == 0 && err == io.EOF {
				return hex.EncodeToString(hash.Sum(nil)), nil
			}
			return "", fmt.Errorf("ogg trailing bytes after EOS")
		}
		if err != nil {
			return "", fmt.Errorf("ogg missing EOS or incomplete page: %w", err)
		}
		flags := header[5]
		if string(header[:4]) != "OggS" || header[4] != 0 || flags&248 != 0 {
			return "", fmt.Errorf("invalid Ogg page header")
		}
		pageSerial, pageSequence := binary.LittleEndian.Uint32(header[14:]), binary.LittleEndian.Uint32(header[18:])
		if first {
			serial = pageSerial
		}
		if pageSerial != serial || pageSequence != sequence || (flags&2 != 0) != first || (flags&1 != 0) != continued {
			return "", fmt.Errorf("ogg stream, sequence or continuation mismatch")
		}
		segments := int(header[26])
		if segments == 0 {
			return "", fmt.Errorf("ogg empty page")
		}
		if _, err := io.ReadFull(file, lacing[:segments]); err != nil {
			return "", fmt.Errorf("incomplete Ogg lacing: %w", err)
		}
		length := 0
		for _, size := range lacing[:segments] {
			length += int(size)
		}
		if _, err := io.ReadFull(file, payload[:length]); err != nil {
			return "", fmt.Errorf("incomplete Ogg payload: %w", err)
		}
		expected := binary.LittleEndian.Uint32(header[22:])
		clear(header[22:26])
		crc := oggCRC(header[:], 0)
		crc = oggCRC(lacing[:segments], crc)
		crc = oggCRC(payload[:length], crc)
		if crc != expected {
			return "", fmt.Errorf("ogg page CRC mismatch")
		}
		binary.LittleEndian.PutUint32(header[22:], expected)
		_, _ = hash.Write(header[:])
		_, _ = hash.Write(lacing[:segments])
		_, _ = hash.Write(payload[:length])
		if first && (length < 8 || string(payload[:8]) != "OpusHead") {
			return "", fmt.Errorf("ogg OpusHead missing")
		}
		continued = lacing[segments-1] == 255
		eos = flags&4 != 0
		if eos && continued {
			return "", fmt.Errorf("ogg EOS has incomplete packet")
		}
		first = false
		sequence++
	}
}

var oggCRCTable = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		c := uint32(i) << 24
		for range 8 {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04c11db7
			} else {
				c <<= 1
			}
		}
		table[i] = c
	}
	return table
}()

func oggCRC(data []byte, crc uint32) uint32 {
	for _, b := range data {
		crc = crc<<8 ^ oggCRCTable[byte(crc>>24)^b]
	}
	return crc
}
