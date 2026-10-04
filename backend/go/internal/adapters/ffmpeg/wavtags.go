package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// repairableTags names the tags this adapter repairs: the user-meaningful fields
// a legacy RIFF INFO chunk carries. Technical keys (the software name, an
// encoder string) are deliberately absent — ffmpeg writes those itself, and
// overriding them would report the source tool as the producer of the output.
var repairableTags = map[string]struct{}{
	"album":   {},
	"artist":  {},
	"comment": {},
	"date":    {},
	"genre":   {},
	"title":   {},
	"track":   {},
}

// rawTag is one reported tag, its value still the container's raw bytes.
type rawTag struct {
	key   string
	value []byte
}

// parseTagPairs walks the `format.tags` object of one ffprobe JSON document.
//
// The values must never pass through a Go string: both Token() and a plain
// Unmarshal into string re-encode invalid bytes as U+FFFD, silently discarding
// exactly what this repair needs. Tokens therefore read only the structure and
// the key names, and each value is captured as raw JSON bytes and unescaped
// byte-for-byte below.
func parseTagPairs(data []byte) ([]rawTag, error) {
	if len(data) == 0 {
		return nil, nil
	}
	document := json.NewDecoder(bytes.NewReader(data))
	if err := expectDelim(document, '{'); err != nil {
		return nil, err
	}
	format, err := objectValue(document, "format")
	if err != nil {
		return nil, err
	}
	if len(format) == 0 {
		return nil, nil
	}
	envelope := json.NewDecoder(bytes.NewReader(format))
	if err = expectDelim(envelope, '{'); err != nil {
		return nil, err
	}
	tags, err := objectValue(envelope, "tags")
	if err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, nil
	}
	object := json.NewDecoder(bytes.NewReader(tags))
	if err = expectDelim(object, '{'); err != nil {
		return nil, err
	}
	var pairs []rawTag
	for object.More() {
		token, err := object.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("tag key is not a string")
		}
		var raw json.RawMessage
		if err = object.Decode(&raw); err != nil {
			return nil, err
		}
		value, err := unquoteJSONString(raw)
		if err != nil {
			continue // a non-string value is not a tag this repair handles
		}
		pairs = append(pairs, rawTag{key: key, value: value})
	}
	return pairs, nil
}

// expectDelim consumes one opening delimiter.
func expectDelim(dec *json.Decoder, want json.Delim) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); !ok || delim != want {
		return fmt.Errorf("expected %q, got %v", want, token)
	}
	return nil
}

// objectValue reads the members of the object the decoder is positioned inside,
// returning the raw value of the first member named want and skipping the rest.
func objectValue(dec *json.Decoder, want string) (json.RawMessage, error) {
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("object key is not a string")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if key == want {
			return raw, nil
		}
	}
	return nil, nil
}

// unquoteJSONString decodes one JSON string literal into the bytes it denotes,
// without validating or repairing UTF-8. ffprobe escapes the backslashes and
// control bytes it must, so a Shift-JIS value whose trailing byte is 0x5c (「表」
// and its neighbours) arrives escaped and only survives if the escape is undone
// byte-for-byte before any decoder sees it.
func unquoteJSONString(raw []byte) ([]byte, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, fmt.Errorf("not a JSON string")
	}
	body := raw[1 : len(raw)-1]
	if bytes.IndexByte(body, '\\') < 0 {
		return body, nil
	}
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); {
		if c := body[i]; c != '\\' {
			out = append(out, c)
			i++
			continue
		}
		if i+1 >= len(body) {
			return nil, fmt.Errorf("truncated escape")
		}
		switch escaped := body[i+1]; escaped {
		case '"', '\\', '/':
			out = append(out, escaped)
			i += 2
		case 'b':
			out = append(out, '\b')
			i += 2
		case 'f':
			out = append(out, '\f')
			i += 2
		case 'n':
			out = append(out, '\n')
			i += 2
		case 'r':
			out = append(out, '\r')
			i += 2
		case 't':
			out = append(out, '\t')
			i += 2
		case 'u':
			codepoint, err := decodeHexEscape(body, i)
			if err != nil {
				return nil, err
			}
			out = utf8.AppendRune(out, codepoint)
			i += 6
		default:
			return nil, fmt.Errorf("unknown escape %q", escaped)
		}
	}
	return out, nil
}

// decodeHexEscape resolves the \uXXXX escape at body[start], joining a surrogate
// pair into the rune it denotes.
func decodeHexEscape(body []byte, start int) (rune, error) {
	point, err := hexCodepoint(body, start+2)
	if err != nil {
		return 0, err
	}
	if !utf16.IsSurrogate(point) || start+12 > len(body) || body[start+6] != '\\' || body[start+7] != 'u' {
		return point, nil
	}
	low, err := hexCodepoint(body, start+8)
	if err != nil {
		return point, nil
	}
	if joined := utf16.DecodeRune(point, low); joined != utf8.RuneError {
		return joined, nil
	}
	return point, nil
}

// hexCodepoint reads the four hex digits at body[start] as one code point.
func hexCodepoint(body []byte, start int) (rune, error) {
	if start+4 > len(body) {
		return 0, fmt.Errorf("truncated \\u escape")
	}
	digits, err := strconv.ParseUint(string(body[start:start+4]), 16, 16)
	if err != nil {
		return 0, err
	}

	return rune(digits), nil
}

// repairedTagArgs turns the reported tags into override arguments.
//
// ffmpeg copies the first match for a key, and a key repeated inside a single
// chunk has already collapsed to its last value by the time ffprobe reports it —
// either way the first reported value is the one that will be written, because
// this reads the same dictionary ffmpeg copies from. Every later value of the
// same key is therefore locked out, repairable or not: reaching for a later
// value would replace the text the output would otherwise carry.
func repairedTagArgs(pairs []rawTag) []string {
	seen := make(map[string]struct{}, len(pairs))
	repaired := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key := strings.ToLower(pair.key)
		if _, ok := repairableTags[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if text, ok := repairTagText(pair.value); ok {
			repaired[key] = text
		}
	}
	if len(repaired) == 0 {
		return nil
	}
	keys := make([]string, 0, len(repaired))
	for key := range repaired {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		args = append(args, "-metadata", key+"="+repaired[key])
	}
	return args
}

// repairTagText returns the text a tag value denotes, when the value is bytes
// that are not valid UTF-8 at all. A value that is already valid UTF-8 is left
// to ffmpeg untouched — including the many tags that are plain ASCII.
func repairTagText(raw []byte) (string, bool) {
	// A NUL cannot travel in an argument vector, and an absent or already-valid
	// value has nothing to repair.
	if len(raw) == 0 || bytes.IndexByte(raw, 0) >= 0 || utf8.Valid(raw) {
		return "", false
	}
	return decodeShiftJIS(raw)
}

// decodeShiftJIS decodes legacy Japanese tag bytes.
//
// ponytail: one fixed guess, Shift-JIS (the code page these tools write), with no
// detection. The decoder reports malformed input by emitting U+FFFD rather than
// by failing, so a result carrying a replacement character is rejected — the
// caller keeps ffmpeg's original value instead of writing fresh mojibake. A
// collection whose tags are EUC-JP would need a real charset pass; nothing here
// has shown one.
func decodeShiftJIS(raw []byte) (string, bool) {
	decoded, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), raw)
	if err != nil {
		return "", false
	}
	text := string(decoded)
	if strings.ContainsRune(text, utf8.RuneError) {
		return "", false
	}
	return text, true
}

// sourceTagHandleArgs reads legacy WAV tags from the same object used to encode.
// sourceTagHandleArgs returns the `-metadata key=value` arguments that repair a source
// file's legacy RIFF INFO tags, or nil when there is nothing to repair.
//
// A WAV written by common Japanese production tools keeps its tags in a RIFF
// INFO chunk as raw Shift-JIS bytes with no encoding declaration. ffmpeg copies
// metadata values verbatim, so those bytes land in the output as invalid UTF-8:
// mojibake in every player, with no error anywhere — exit code 0, empty stderr.
// ffprobe already reports the value ffmpeg will pick, so this repairs only that
// value's encoding and never re-ranks the candidates: a source that currently
// reads correctly must keep reading correctly.
//
// Only RIFF/WAVE sources are examined. A source whose tags cannot be read is
// converted exactly as before — repairing metadata is best-effort, and must
// never turn a working conversion into a failed one. A canceled context is not
// such a failure: it is the caller's own signal and is returned.
func (f Encoder) sourceTagHandleArgs(ctx context.Context, file *os.File) ([]string, error) {
	var header [12]byte
	if _, err := file.ReadAt(header[:], 0); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return nil, nil
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	_, probe := f.paths()
	cmd := exec.CommandContext(
		ctx,
		probe,
		"-v",
		"error",
		"-show_entries",
		"format_tags",
		"-of",
		"json=string_validation=ignore",
		"-fd",
		"0",
		"fd:",
	)
	cmd.Stdin = file
	data, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	pairs, err := parseTagPairs(data)
	if err != nil {
		return nil, nil
	}
	return repairedTagArgs(pairs), nil
}
