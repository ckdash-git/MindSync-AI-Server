package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/middleware"
	"github.com/ckdash-git/MindSync-AI-Server/internal/service"
	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
	"github.com/go-chi/chi/v5"
)

// AuthHandler handles authentication HTTP endpoints.
type AuthHandler struct {
	authService *service.AuthService
	log         *logger.Logger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService, log *logger.Logger) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		log:         log,
	}
}

// RegisterRoutes registers auth routes on the given router.
func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.With(middleware.RequireAuth).Post("/logout", h.Logout)
	})
}

// Register handles user registration.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var input service.RegisterInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	result, err := h.authService.Register(r.Context(), input)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.Created(w, result)
}

// Login handles user authentication.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input service.LoginInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	result, err := h.authService.Login(r.Context(), input)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, result)
}

// Refresh handles token refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	result, err := h.authService.RefreshTokens(r.Context(), input.RefreshToken)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, result)
}

// Logout handles user logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	if err := h.authService.Logout(r.Context(), claims.UserID); err != nil {
		h.log.ErrorContext(r.Context(), "logout failed", "error", err)
		response.InternalError(w)
		return
	}

	response.NoContent(w)
}

// handleError maps domain errors to HTTP responses.
func (h *AuthHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		response.Unauthorized(w, "invalid email or password")
	case errors.Is(err, domain.ErrUserAlreadyExists):
		response.Conflict(w, "a user with this email already exists")
	case errors.Is(err, domain.ErrInvalidToken):
		response.Unauthorized(w, "invalid or expired token")
	case errors.Is(err, domain.ErrSessionExpired):
		response.Unauthorized(w, "session expired, please login again")
	case errors.Is(err, domain.ErrInvalidInput):
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			response.BadRequest(w, ve.Message)
		} else {
			response.BadRequest(w, "invalid input")
		}
	default:
		h.log.ErrorContext(r.Context(), "unhandled error", "error", err)
		response.InternalError(w)
	}
}
