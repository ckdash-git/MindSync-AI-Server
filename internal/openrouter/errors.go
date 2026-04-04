package openrouter

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
)

// Client-side errors.
var (
	ErrStreamClosed  = errors.New("stream closed")
	ErrStreamTimeout = errors.New("stream timeout")
)

// MapHTTPError maps an HTTP status code and error response to a domain error.
func MapHTTPError(statusCode int, errResp *ErrorResponse) error {
	msg := "unknown error"
	if errResp != nil {
		msg = errResp.Error.Message
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", domain.ErrUnauthorized, msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", domain.ErrRateLimited, msg)
	case http.StatusBadRequest:
		return fmt.Errorf("%w: %s", domain.ErrInvalidInput, msg)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", domain.ErrModelUnavailable, msg)
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return fmt.Errorf("%w: %s", domain.ErrUpstreamTimeout, msg)
	default:
		if statusCode >= 500 {
			return fmt.Errorf("%w: [%d] %s", domain.ErrUpstreamError, statusCode, msg)
		}
		return fmt.Errorf("%w: [%d] %s", domain.ErrUpstreamError, statusCode, msg)
	}
}
