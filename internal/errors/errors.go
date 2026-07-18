// Package errors defines the domain errors for the board.
//
// The CLI renders them as "error: <message>" and exits non-zero. Keeping a small
// typed error set means the core stays a plain library.
package errors

import stderrors "errors"

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

// As reports whether err is (or wraps) a BoardError, returning it if so, so
// a %w-wrapped domain error keeps its code and exit behaviour.
func As(err error) (BoardError, bool) {
	var be BoardError
	if stderrors.As(err, &be) {
		return be, true
	}
	return nil, false
}

// ExitCode maps an error to the universal CLI exit-code contract, identical
// in every output mode: 0 success, 1 internal (or user-aborted),
// 2 invalid/usage, 3 not_found, 4 conflict.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if be, ok := As(err); ok {
		switch be.Code() {
		case "invalid":
			return 2
		case "not_found":
			return 3
		case "conflict":
			return 4
		}
	}
	return 1
}
