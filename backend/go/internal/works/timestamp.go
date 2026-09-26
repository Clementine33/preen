package works

import "time"

// TimestampFormat is the single on-disk form for a catalog timestamp: UTC,
// always nine fractional digits, always the same width. Fixed width is what
// makes the text order the time order, so a range scan can be answered by an
// index instead of by julianday().
//
// The repository's older timeFormat (time.RFC3339Nano) is variable precision:
// Go drops trailing zeros, so "…00Z" sorts after "…00.1Z" as text even though
// it is earlier. New catalog columns never use it.
const TimestampFormat = "2006-01-02T15:04:05.000000000Z"

// DateFormat is the on-disk form of a calendar date.
const DateFormat = "2006-01-02"

// parseTimestamp reads the catalog's fixed-width UTC form and refuses every
// other shape, including the RFC3339 variant without a fractional part.
func parseTimestamp(s string) (time.Time, error) { return time.Parse(TimestampFormat, s) }
