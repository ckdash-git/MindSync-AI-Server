package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/middleware"
	"github.com/ckdash-git/MindSync-AI-Server/internal/service"
	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ChatHandler handles chat HTTP endpoints.
type ChatHandler struct {
	chatService *service.ChatService
	log         *logger.Logger
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(chatService *service.ChatService, log *logger.Logger) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		log:         log,
	}
}

// RegisterRoutes registers chat routes on the given router.
func (h *ChatHandler) RegisterRoutes(r chi.Router) {
	r.Route("/chats", func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Post("/", h.CreateChat)
		r.Get("/", h.ListChats)
		r.Get("/{chatID}", h.GetChat)
		r.Delete("/{chatID}", h.DeleteChat)
		r.Post("/{chatID}/messages", h.SendMessage)
	})
}

// CreateChat handles chat creation.
func (h *ChatHandler) CreateChat(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	var input service.CreateChatInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	chat, err := h.chatService.CreateChat(r.Context(), claims.UserID, input)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.Created(w, chat)
}

// ListChats handles listing user's chats.
func (h *ChatHandler) ListChats(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	chats, total, err := h.chatService.ListChats(r.Context(), claims.UserID, page, perPage)
	if err != nil {
		h.log.ErrorContext(r.Context(), "listing chats", "error", err)
		response.InternalError(w)
		return
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 20
	}

	response.Paginated(w, chats, page, perPage, total)
}

// GetChat handles retrieving a single chat with messages.
func (h *ChatHandler) GetChat(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		response.BadRequest(w, "invalid chat ID")
		return
	}

	chat, messages, err := h.chatService.GetChat(r.Context(), claims.UserID, chatID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, map[string]interface{}{
		"chat":     chat,
		"messages": messages,
	})
}

// DeleteChat handles chat deletion.
func (h *ChatHandler) DeleteChat(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		response.BadRequest(w, "invalid chat ID")
		return
	}

	if err := h.chatService.DeleteChat(r.Context(), claims.UserID, chatID); err != nil {
		h.handleError(w, r, err)
		return
	}

	response.NoContent(w)
}

// SendMessage handles sending a message in a chat.
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		response.BadRequest(w, "invalid chat ID")
		return
	}

	var input service.SendMessageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	result, err := h.chatService.SendMessage(r.Context(), claims.UserID, chatID, input)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, result)
}

// handleError maps domain errors to HTTP responses.
func (h *ChatHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		response.NotFound(w, "resource not found")
	case errors.Is(err, domain.ErrForbidden):
		response.Forbidden(w, "you do not have access to this resource")
	case errors.Is(err, domain.ErrInvalidInput):
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			response.BadRequest(w, ve.Message)
		} else {
			response.BadRequest(w, "invalid input")
		}
	case errors.Is(err, domain.ErrUpstreamTimeout):
		response.GatewayTimeout(w)
	case errors.Is(err, domain.ErrUpstreamError):
		response.Error(w, http.StatusBadGateway, "UPSTREAM_ERROR", "AI service error")
	case errors.Is(err, domain.ErrRateLimited):
		response.TooManyRequests(w, "AI service rate limit exceeded")
	default:
		h.log.ErrorContext(r.Context(), "unhandled error in chat handler", "error", err)
		response.InternalError(w)
	}
}
