package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/security"
	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
)

// claimsKey is the context key for JWT claims.
type claimsKey struct{}

// AuthMiddleware validates JWT tokens from the Authorization header.
type AuthMiddleware struct {
	jwt *security.JWTManager
	log *logger.Logger
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(jwt *security.JWTManager, log *logger.Logger) *AuthMiddleware {
	return &AuthMiddleware{jwt: jwt, log: log}
}

// Handler returns the middleware handler function.
// It validates the JWT and injects claims + user_id into the context.
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			response.Unauthorized(w, "missing authorization token")
			return
		}

		claims, err := m.jwt.ValidateAccessToken(token)
		if err != nil {
			m.log.WarnContext(r.Context(), "invalid token", "error", err)
			response.Unauthorized(w, "invalid or expired token")
			return
		}

		// Inject claims into context
		ctx := context.WithValue(r.Context(), claimsKey{}, claims)

		// Inject user_id into context for logging correlation
		ctx = logger.WithUserID(ctx, claims.UserID.String())

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetClaims extracts JWT claims from the request context.
func GetClaims(ctx context.Context) (*security.Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(*security.Claims)
	return claims, ok
}

// RequireAuth is a standalone middleware function for use with chi's With().
// It must be initialized by SetAuthMiddleware before use.
var RequireAuth func(http.Handler) http.Handler

// SetAuthMiddleware initializes the package-level RequireAuth middleware.
func SetAuthMiddleware(m *AuthMiddleware) {
	RequireAuth = m.Handler
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}
