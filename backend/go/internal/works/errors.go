package works

import "errors"

// Kind is the class of a refusal, which the HTTP layer maps to a status.
type Kind int

// The refusal classes.
const (
	KindInvalid Kind = iota
	KindNotFound
	KindConflict
	KindInternal
)

// Stable codes the catalog answers with.
const (
	CodeInvalidArgument   = "INVALID_ARGUMENT"
	CodeWorkNotFound      = "WORK_NOT_FOUND"
	CodeWorkInUse         = "WORK_IN_USE"
	CodeInvalidPage       = "INVALID_PAGE"
	CodeInvalidCursor     = "INVALID_CURSOR"
	CodeCursorMismatch    = "CURSOR_QUERY_MISMATCH"
	CodeMatchModeRequired = "MATCH_MODE_REQUIRED"
	CodeInvalidTagName    = "INVALID_TAG_NAME"
	CodeUserTagExists     = "USER_TAG_EXISTS"
	CodeUserTagNotFound   = "USER_TAG_NOT_FOUND"
	CodeTimestampFormat   = "INVALID_TIMESTAMP"
)

// Error is one refusal, carrying the stable code a caller branches on.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Details []string
}

// NewError builds a refusal.
func NewError(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Invalid builds a 400 refusal.
func Invalid(code, message string) *Error { return NewError(KindInvalid, code, message) }

// NotFound builds a 404 refusal.
func NotFound(code, message string) *Error { return NewError(KindNotFound, code, message) }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// AsError extracts a refusal from an error chain.
func AsError(err error) (*Error, bool) {
	return errors.AsType[*Error](err)
}
