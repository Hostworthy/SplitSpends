// Package errs defines the typed domain errors that services return.
// Transport layers map these to protocol-specific responses; the domain never
// knows about HTTP status codes.
package errs

import (
	"errors"
	"fmt"
)

// Kind classifies a domain error for transport mapping.
type Kind string

const (
	KindValidation   Kind = "VALIDATION_ERROR"
	KindNotFound     Kind = "NOT_FOUND"
	KindUnauthorized Kind = "UNAUTHORIZED"
	KindForbidden    Kind = "FORBIDDEN"
	KindConflict     Kind = "CONFLICT"
	KindRateLimited  Kind = "RATE_LIMITED"
	KindUnavailable  Kind = "AI_DISABLED"
)

// Error is a domain error with a human-readable message.
type Error struct {
	Kind    Kind
	Message string
	Field   string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// Validation reports invalid input. Field is the offending JSON field, if any.
func Validation(field, message string) *Error {
	return &Error{Kind: KindValidation, Field: field, Message: message}
}

// NotFound reports a missing entity.
func NotFound(entity string) *Error {
	return &Error{Kind: KindNotFound, Message: entity + " not found."}
}

// Unauthorized reports missing or invalid credentials.
func Unauthorized(message string) *Error {
	return &Error{Kind: KindUnauthorized, Message: message}
}

// Forbidden reports an authenticated caller without access.
func Forbidden(message string) *Error {
	return &Error{Kind: KindForbidden, Message: message}
}

// Conflict reports a state conflict such as unsettled balances.
func Conflict(message string) *Error {
	return &Error{Kind: KindConflict, Message: message}
}

// RateLimited reports too many attempts.
func RateLimited(message string) *Error {
	return &Error{Kind: KindRateLimited, Message: message}
}

// Unavailable reports an optional capability the host did not enable.
func Unavailable(message string) *Error {
	return &Error{Kind: KindUnavailable, Message: message}
}

// As extracts a domain error from err.
func As(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// Is reports whether err is a domain error of the given kind.
func Is(err error, kind Kind) bool {
	e, ok := As(err)
	return ok && e.Kind == kind
}
