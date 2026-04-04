package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
)

// Recovery returns middleware that recovers from panics and logs the stack trace.
// Returns 500 to the client without exposing internal details.
func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					stack := debug.Stack()
					log.ErrorContext(r.Context(), "panic recovered",
						"error", err,
						"stack", string(stack),
					)
					response.InternalError(w)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
