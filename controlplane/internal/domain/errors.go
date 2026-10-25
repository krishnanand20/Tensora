package domain

import (
	"errors"
	"fmt"
)

// Code classifies an expected, user-facing failure. The API maps codes to HTTP statuses.
type Code string

const (
	CodeInvalid           Code = "invalid_request"
	CodeUnauthenticated   Code = "unauthenticated"
	CodeForbidden         Code = "forbidden"
	CodeNotFound          Code = "not_found"
	CodeConflict          Code = "conflict"
	CodeQuotaExceeded     Code = "quota_exceeded"
	CodeInvalidTransition Code = "invalid_transition"
	CodeTooManyRequests   Code = "too_many_requests"
)

// Error is an expected failure whose message is safe to show the caller.
// Any other error is internal: logged, and reported to the caller as a generic 500.
type Error struct {
	Code    Code
	Message string
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

func Errorf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// CodeOf returns the Code of the first *Error in err's chain.
func CodeOf(err error) (Code, bool) {
	var de *Error
	if errors.As(err, &de) {
		return de.Code, true
	}
	return "", false
}

func IsNotFound(err error) bool {
	c, ok := CodeOf(err)
	return ok && c == CodeNotFound
}
