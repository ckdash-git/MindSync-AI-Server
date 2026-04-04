package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Context keys for request-scoped values.
type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	UserIDKey    contextKey = "user_id"
)

// Logger wraps slog.Logger with privacy-safe defaults.
type Logger struct {
	*slog.Logger
}

// New creates a structured JSON logger with the given level.
// It injects request_id and user_id from context automatically.
func New(level string) *Logger {
	lvl := parseLevel(level)

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// ISO 8601 timestamps
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format(time.RFC3339Nano))
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	contextHandler := &ContextHandler{inner: handler}
	l := slog.New(contextHandler)

	return &Logger{Logger: l}
}

// NewWithWriter creates a logger writing to a custom writer (useful for testing).
func NewWithWriter(level string, w io.Writer) *Logger {
	lvl := parseLevel(level)

	opts := &slog.HandlerOptions{Level: lvl}
	handler := slog.NewJSONHandler(w, opts)
	contextHandler := &ContextHandler{inner: handler}
	l := slog.New(contextHandler)

	return &Logger{Logger: l}
}

// WithRequestID returns a context with the given request ID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithUserID returns a context with the given user ID.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetRequestID extracts request_id from context.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
}

// GetUserID extracts user_id from context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}

// ContextHandler wraps a slog.Handler to inject request_id and user_id
// from the context into every log record automatically.
type ContextHandler struct {
	inner slog.Handler
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *ContextHandler) Handle(ctx context.Context, record slog.Record) error {
	// Inject request_id from context
	if requestID := GetRequestID(ctx); requestID != "" {
		record.AddAttrs(slog.String("request_id", requestID))
	}

	// Inject user_id from context (if authenticated)
	if userID := GetUserID(ctx); userID != "" {
		record.AddAttrs(slog.String("user_id", userID))
	}

	return h.inner.Handle(ctx, record)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{inner: h.inner.WithGroup(name)}
}

// MaskSecret masks a secret string for safe logging.
// Shows first 4 and last 4 characters if long enough.
func MaskSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
