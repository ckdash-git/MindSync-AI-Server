package middleware

import (
	"net/http"
	"time"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
)

// Logging returns middleware that logs request/response details.
// NEVER logs request body (could contain prompts or PII).
// Always includes request_id, includes user_id when authenticated.
func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(ww, r)

			latency := time.Since(start)

			// Log at appropriate level based on status code
			// request_id and user_id are automatically injected by the ContextHandler
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"latency_ms", latency.Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			}

			switch {
			case ww.status >= 500:
				log.ErrorContext(r.Context(), "request completed", attrs...)
			case ww.status >= 400:
				log.WarnContext(r.Context(), "request completed", attrs...)
			default:
				log.InfoContext(r.Context(), "request completed", attrs...)
			}
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for SSE support.
func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
