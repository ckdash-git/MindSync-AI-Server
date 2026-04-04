package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
)

// TimeoutConfig holds timeout settings.
type TimeoutConfig struct {
	Default time.Duration
	Stream  time.Duration
}

// Timeout returns middleware that applies a request timeout.
func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})
			go func() {
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				// Request completed normally
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					response.GatewayTimeout(w)
				}
			}
		})
	}
}

// StreamTimeout returns middleware with a longer timeout for streaming endpoints.
func StreamTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return Timeout(timeout)
}
