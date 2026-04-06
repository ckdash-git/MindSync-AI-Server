package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/middleware"
	"github.com/ckdash-git/MindSync-AI-Server/internal/service"
	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// FacadeHandler provides simplified, single-request endpoints that compose
// existing service operations into convenient client-facing APIs.
type FacadeHandler struct {
	chatService   *service.ChatService
	streamService *service.StreamService
	defaultModel  string
	rateLimiter   *middleware.RateLimiter
	log           *logger.Logger
}

// NewFacadeHandler creates a new FacadeHandler.
func NewFacadeHandler(chatService *service.ChatService, streamService *service.StreamService, defaultModel string, rateLimiter *middleware.RateLimiter, log *logger.Logger) *FacadeHandler {
	return &FacadeHandler{
		chatService:   chatService,
		streamService: streamService,
		defaultModel:  defaultModel,
		rateLimiter:   rateLimiter,
		log:           log,
	}
}

// RegisterRoutes registers the simplified façade routes on the given router.
func (h *FacadeHandler) RegisterRoutes(r chi.Router) {
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("chat")).Post("/chat", h.SimpleChat)
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("stream")).Post("/chat/stream", h.StreamChat)
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("chat")).Post("/explain", h.Explain)
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("council")).Post("/ai-council", h.AICouncil)
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("chat")).Post("/session/summary", h.SessionSummary)
}

// ── Request / Response Types ────────────────────────────────────────────────

type SimpleChatRequest struct {
	ChatID  string `json:"chat_id,omitempty"`
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
}

type SimpleChatResponse struct {
	ChatID   string `json:"chat_id"`
	Response string `json:"response"`
	Model    string `json:"model"`
	Tokens   int    `json:"tokens_used"`
}

type StreamChatRequest struct {
	ChatID  string `json:"chat_id,omitempty"`
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
}

type ExplainRequest struct {
	Topic string `json:"topic"`
	Model string `json:"model,omitempty"`
}

type ExplainResponse struct {
	Topic       string `json:"topic"`
	Explanation string `json:"explanation"`
	Model       string `json:"model"`
	Tokens      int    `json:"tokens_used"`
}

type AICouncilRequest struct {
	Question string   `json:"question"`
	Models   []string `json:"models,omitempty"`
}

type SessionSummaryRequest struct {
	ChatID string `json:"chat_id"`
	Model  string `json:"model,omitempty"`
}

type SessionSummaryResponse struct {
	ChatID  string `json:"chat_id"`
	Summary string `json:"summary"`
	Model   string `json:"model"`
	Tokens  int    `json:"tokens_used"`
}

// ── Shared Helpers ──────────────────────────────────────────────────────────

func (h *FacadeHandler) extractAPIKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	apiKey := r.Header.Get("X-OpenRouter-Key")
	if apiKey == "" {
		response.Error(w, http.StatusUnauthorized, "MISSING_API_KEY", "X-OpenRouter-Key header is required")
		return "", false
	}
	return apiKey, true
}

func parseOptionalChatID(idStr string) (*uuid.UUID, error) {
	if idStr == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (h *FacadeHandler) SimpleChat(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	apiKey, ok := h.extractAPIKey(w, r)
	if !ok {
		return
	}

	var req SimpleChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.Message == "" {
		response.BadRequest(w, "message is required")
		return
	}

	chatID, err := parseOptionalChatID(req.ChatID)
	if err != nil {
		response.BadRequest(w, "invalid chat_id format")
		return
	}

	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	result, newChatID, err := h.chatService.FacadeChat(r.Context(), claims.UserID, service.FacadeChatInput{
		ChatID:  chatID,
		Message: req.Message,
		Model:   model,
		APIKey:  apiKey,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, SimpleChatResponse{
		ChatID:   newChatID.String(),
		Response: result.AssistantMessage.Content,
		Model:    result.AssistantMessage.Model,
		Tokens:   result.AssistantMessage.TokensUsed,
	})
}

func (h *FacadeHandler) StreamChat(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	apiKey, ok := h.extractAPIKey(w, r)
	if !ok {
		return
	}

	var req StreamChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}
	if req.Message == "" {
		response.BadRequest(w, "message is required")
		return
	}

	chatID, err := parseOptionalChatID(req.ChatID)
	if err != nil {
		response.BadRequest(w, "invalid chat_id format")
		return
	}

	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.log.ErrorContext(r.Context(), "response writer does not support flushing")
		response.InternalError(w)
		return
	}

	chunkCh, errCh, finalChatID, err := h.chatService.FacadeStream(r.Context(), claims.UserID, service.FacadeStreamInput{
		ChatID:  chatID,
		Message: req.Message,
		Model:   model,
		APIKey:  apiKey,
	}, h.streamService)

	if err != nil {
		header := map[string]string{"message": "failed to start stream"}
		headerData, _ := json.Marshal(header)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", headerData)
		flusher.Flush()
		h.handleError(w, r, err)
		return
	}

	// First chunk will carry the chat_id so client knows it
	initData, _ := json.Marshal(map[string]string{"chat_id": finalChatID})
	fmt.Fprintf(w, "event: init\ndata: %s\n\n", initData)
	flusher.Flush()

	for {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

		case err, ok := <-errCh:
			if ok && err != nil {
				h.log.ErrorContext(r.Context(), "stream error", "error", err)
				errData, _ := json.Marshal(map[string]string{"message": "stream error"})
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", errData)
				flusher.Flush()
			}
			return

		case <-r.Context().Done():
			h.log.InfoContext(r.Context(), "client disconnected during stream")
			return
		}
	}
}

func (h *FacadeHandler) Explain(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	apiKey, ok := h.extractAPIKey(w, r)
	if !ok {
		return
	}

	var req ExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.Topic == "" {
		response.BadRequest(w, "topic is required")
		return
	}

	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	result, err := h.chatService.Explain(r.Context(), claims.UserID, service.ExplainInput{
		Topic:  req.Topic,
		Model:  model,
		APIKey: apiKey,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, ExplainResponse{
		Topic:       req.Topic,
		Explanation: result.AssistantMessage.Content,
		Model:       result.AssistantMessage.Model,
		Tokens:      result.AssistantMessage.TokensUsed,
	})
}

func (h *FacadeHandler) AICouncil(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	apiKey, ok := h.extractAPIKey(w, r)
	if !ok {
		return
	}

	var req AICouncilRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.Question == "" {
		response.BadRequest(w, "question is required")
		return
	}

	models := req.Models
	if len(models) == 0 {
		models = []string{
			"openai/gpt-4o",
			"anthropic/claude-3.5-sonnet",
			"google/gemini-pro",
		}
	}

	result, err := h.chatService.AICouncil(r.Context(), claims.UserID, service.AICouncilInput{
		Question: req.Question,
		Models:   models,
		APIKey:   apiKey,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, result) // Wraps successfully with response format
}

func (h *FacadeHandler) SessionSummary(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
		return
	}

	apiKey, ok := h.extractAPIKey(w, r)
	if !ok {
		return
	}

	var req SessionSummaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	if req.ChatID == "" {
		response.BadRequest(w, "chat_id is required")
		return
	}

	chatID, err := uuid.Parse(req.ChatID)
	if err != nil {
		response.BadRequest(w, "invalid chat_id format")
		return
	}

	result, err := h.chatService.SessionSummary(r.Context(), claims.UserID, service.SessionSummaryInput{
		ChatID: chatID,
		Model:  req.Model,
		APIKey: apiKey,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, SessionSummaryResponse{
		ChatID:  req.ChatID,
		Summary: result.AssistantMessage.Content,
		Model:   result.AssistantMessage.Model,
		Tokens:  result.AssistantMessage.TokensUsed,
	})
}

// handleError maps domain errors to HTTP responses.
func (h *FacadeHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
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
		h.log.ErrorContext(r.Context(), "unhandled error in facade handler", "error", err)
		response.InternalError(w)
	}
}

