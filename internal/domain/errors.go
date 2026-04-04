package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors for the domain layer.
var (
	// Auth errors
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrSessionExpired     = errors.New("session expired")

	// Resource errors
	ErrNotFound     = errors.New("resource not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")

	// Validation errors
	ErrInvalidInput = errors.New("invalid input")

	// Rate limiting
	ErrRateLimited = errors.New("rate limit exceeded")

	// Upstream errors
	ErrUpstreamTimeout  = errors.New("upstream service timeout")
	ErrUpstreamError    = errors.New("upstream service error")
	ErrModelUnavailable = errors.New("requested model is unavailable")
)

// ValidationError wraps ErrInvalidInput with field-level details.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %q: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidInput
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}
