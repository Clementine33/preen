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
	CodeInvalidArgument      = "INVALID_ARGUMENT"
	CodeWorkNotFound         = "WORK_NOT_FOUND"
	CodeWorkInUse            = "WORK_IN_USE"
	CodeInvalidPage          = "INVALID_PAGE"
	CodeMatchModeRequired    = "MATCH_MODE_REQUIRED"
	CodeInvalidTagName       = "INVALID_TAG_NAME"
	CodeUserTagExists        = "USER_TAG_EXISTS"
	CodeUserTagNotFound      = "USER_TAG_NOT_FOUND"
	CodeTimestampFormat      = "INVALID_TIMESTAMP"
	CodeInvalidPreferences   = "INVALID_PREFERENCES"
	CodeInvalidOutboundProxy = "INVALID_OUTBOUND_PROXY"
)

// Error is one refusal, carrying the stable code a caller branches on. Cause is
// the unexpected failure behind an internal refusal: for the log, never the
// response.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Details []string
	Cause   error
}

// NewError builds a refusal.
func NewError(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Invalid builds a 400 refusal.
func Invalid(code, message string) *Error { return NewError(KindInvalid, code, message) }

// NotFound builds a 404 refusal.
func NotFound(code, message string) *Error { return NewError(KindNotFound, code, message) }

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// AsError extracts a refusal from an error chain.
func AsError(err error) (*Error, bool) {
	return errors.AsType[*Error](err)
}
