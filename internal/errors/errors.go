// Package errors defines the domain errors for the board.
//
// Surfaces translate these: the CLI renders them as "error: <message>" and the
// MCP layer turns them into plain tool errors. Keeping them out of any HTTP
// framework means the core stays a plain library.
package errors

// BoardError is the interface implemented by all board errors. Each carries a
// short machine code and a human message.
type BoardError interface {
	error
	Code() string
	Message() string
}

// boardError is the shared implementation behind the concrete error types.
type boardError struct {
	code    string
	message string
}

func (e *boardError) Error() string   { return e.message }
func (e *boardError) Code() string    { return e.code }
func (e *boardError) Message() string { return e.message }

// NotFound: a task or board could not be found.
func NotFound(message string) BoardError {
	return &boardError{code: "not_found", message: message}
}

// Conflict: the operation is illegal in the current state (e.g. a bad transition).
func Conflict(message string) BoardError {
	return &boardError{code: "conflict", message: message}
}

// Invalid: the request itself is malformed (e.g. an unknown status).
func Invalid(message string) BoardError {
	return &boardError{code: "invalid", message: message}
}

// As reports whether err is a BoardError, returning it if so.
func As(err error) (BoardError, bool) {
	be, ok := err.(BoardError)
	return be, ok
}
