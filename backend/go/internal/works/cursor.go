package works

import (
	"strconv"
	"strings"
)

// cursorVersion is the format tag. It is part of the value so a future format
// can be refused explicitly instead of misread.
const cursorVersion = "v1"

// cursorFields is the number of "|"-separated fields a cursor carries.
const cursorFields = 7

// Cursor is one page position: the sort position of the last row a caller saw,
// plus the fingerprint of the query that produced it.
type Cursor struct {
	Sort      SortField
	Direction Direction
	Digest    string
	// NullRank is 0 while rows with a sort value remain and 1 once the trailing
	// block of unknown values has been entered. Only a Nullable field can be
	// unknown.
	NullRank int
	// SortValue is the row's sort key in its stored form; it is empty inside
	// the unknown block, where the work number alone orders the rows.
	SortValue string
	// WorkID is the row's canonical number: the tie-break that keeps the order
	// total when two rows share a sort value.
	WorkID WorkCode
}

// Encode renders the cursor. None of the fields can contain "|": the digest is
// hex, the enums are fixed words, and the value and the number are drawn from
// fixed alphabets.
func (c Cursor) Encode() string {
	return strings.Join([]string{
		cursorVersion,
		string(c.Sort),
		string(c.Direction),
		c.Digest,
		strconv.Itoa(c.NullRank),
		c.SortValue,
		string(c.WorkID),
	}, "|")
}

// DecodeCursor reads a cursor, refusing every shape it did not write.
func DecodeCursor(raw string) (Cursor, *Error) {
	var cursor Cursor
	fields := strings.Split(raw, "|")
	if len(fields) != cursorFields || fields[0] != cursorVersion {
		return cursor, Invalid(CodeInvalidCursor, "the cursor is malformed; start from the first page")
	}
	cursor.Sort = SortField(fields[1])
	cursor.Direction = Direction(fields[2])
	cursor.Digest = fields[3]
	cursor.SortValue = fields[5]
	if !cursor.Sort.Valid() || !cursor.Direction.Valid() {
		return cursor, Invalid(CodeInvalidCursor, "the cursor names an unknown order")
	}
	rank, err := strconv.Atoi(fields[4])
	if err != nil || (rank != 0 && rank != 1) {
		return cursor, Invalid(CodeInvalidCursor, "the cursor has a malformed position")
	}
	cursor.NullRank = rank
	if rank == 1 && !cursor.Sort.Nullable() {
		return cursor, Invalid(CodeInvalidCursor, "the cursor has a malformed position")
	}
	// Inside the unknown block there is no value to carry; anywhere else a
	// missing one is a cursor this build did not write.
	if (rank == 1) != (cursor.SortValue == "") {
		return cursor, Invalid(CodeInvalidCursor, "the cursor has no sort value")
	}
	if cursor.Sort == SortByRating && rank == 0 {
		if rating, err := strconv.Atoi(cursor.SortValue); err != nil || rating < 1 || rating > 10 {
			return cursor, Invalid(CodeInvalidCursor, "the cursor has a malformed position")
		}
	}
	workID, ok := ParseWorkCode(fields[6])
	if !ok {
		return cursor, Invalid(CodeInvalidCursor, "the cursor names no work number")
	}
	cursor.WorkID = workID
	return cursor, nil
}

// Matches rejects a cursor that belongs to a different query, which is what a
// caller gets after changing a filter or the ordering: the old cursor's idea
// of "the rows after this one" no longer describes the same sequence.
func (c Cursor) Matches(f Filter) *Error {
	if c.Sort != f.Sort || c.Direction != f.Direction || c.Digest != f.Digest() {
		return Invalid(CodeCursorMismatch, "the cursor belongs to a different query; start from the first page")
	}
	return nil
}
