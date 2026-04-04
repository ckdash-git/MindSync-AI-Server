package middleware

import (
	"context"
	"net/http"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/google/uuid"
)

// requestIDKey is the context key for request ID.
type requestIDKey struct{}

// RequestID injects a unique request ID into the context and response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use client-provided request ID if present, otherwise generate one
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}

		// Set response header
		w.Header().Set("X-Request-ID", reqID)

		// Inject into context for downstream use
		ctx := context.WithValue(r.Context(), requestIDKey{}, reqID)
		ctx = logger.WithRequestID(ctx, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}
