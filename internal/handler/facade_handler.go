package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/middleware"
	"github.com/ckdash-git/MindSync-AI-Server/internal/service"
	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// defaultCouncilModels defines the default models for the AI council endpoint.
var defaultCouncilModels = []string{
	"openai/gpt-4o",
	"anthropic/claude-3.5-sonnet",
	"google/gemini-pro",
}

// FacadeHandler provides simplified, single-request endpoints that compose
// existing ChatService operations into convenient client-facing APIs.
type FacadeHandler struct {
	chatService  *service.ChatService
	defaultModel string
	rateLimiter  *middleware.RateLimiter
	log          *logger.Logger
}

// NewFacadeHandler creates a new FacadeHandler.
func NewFacadeHandler(chatService *service.ChatService, defaultModel string, rateLimiter *middleware.RateLimiter, log *logger.Logger) *FacadeHandler {
	return &FacadeHandler{
		chatService:  chatService,
		defaultModel: defaultModel,
		rateLimiter:  rateLimiter,
		log:          log,
	}
}

// RegisterRoutes registers the simplified façade routes on the given router.
func (h *FacadeHandler) RegisterRoutes(r chi.Router) {
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("chat")).Post("/chat", h.SimpleChat)
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("chat")).Post("/explain", h.Explain)
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("chat")).Post("/ai-council", h.AICouncil)
	r.With(middleware.RequireAuth, h.rateLimiter.Handler("chat")).Post("/session/summary", h.SessionSummary)
}

// ── Request / Response Types ────────────────────────────────────────────────

// SimpleChatRequest is the request body for POST /api/v1/chat.
type SimpleChatRequest struct {
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"api_key"`
}

// SimpleChatResponse is the response for POST /api/v1/chat.
type SimpleChatResponse struct {
	ChatID   string `json:"chat_id"`
	Response string `json:"response"`
	Model    string `json:"model"`
	Tokens   int    `json:"tokens_used"`
}

// ExplainRequest is the request body for POST /api/v1/explain.
type ExplainRequest struct {
	Topic  string `json:"topic"`
	Model  string `json:"model,omitempty"`
	APIKey string `json:"api_key"`
}

// ExplainResponse is the response for POST /api/v1/explain.
type ExplainResponse struct {
	Topic       string `json:"topic"`
	Explanation string `json:"explanation"`
	Model       string `json:"model"`
	Tokens      int    `json:"tokens_used"`
}

// AICouncilRequest is the request body for POST /api/v1/ai-council.
type AICouncilRequest struct {
	Question string   `json:"question"`
	Models   []string `json:"models,omitempty"`
	APIKey   string   `json:"api_key"`
}

// CouncilOpinion holds a single model's response within the council.
type CouncilOpinion struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Tokens   int    `json:"tokens_used"`
	Error    string `json:"error,omitempty"`
}

// AICouncilResponse is the response for POST /api/v1/ai-council.
type AICouncilResponse struct {
	Question string           `json:"question"`
	Opinions []CouncilOpinion `json:"opinions"`
}

// SessionSummaryRequest is the request body for POST /api/v1/session/summary.
type SessionSummaryRequest struct {
	ChatID string `json:"chat_id"`
	Model  string `json:"model,omitempty"`
	APIKey string `json:"api_key"`
}

// SessionSummaryResponse is the response for POST /api/v1/session/summary.
type SessionSummaryResponse struct {
	ChatID  string `json:"chat_id"`
	Summary string `json:"summary"`
	Model   string `json:"model"`
	Tokens  int    `json:"tokens_used"`
}

// ── Handlers ────────────────────────────────────────────────────────────────

// SimpleChat handles POST /api/v1/chat.
// It creates a transient chat, sends the user message, and returns the AI response.
func (h *FacadeHandler) SimpleChat(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
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
	if req.APIKey == "" {
		response.BadRequest(w, "api_key is required")
		return
	}

	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	// Auto-generate a title from the first 50 characters of the message.
	title := truncate(req.Message, 50)

	// Step 1: Create a chat and send the first message atomically.
	chat, result, err := h.chatService.CreateChatAndSendFirstMessage(r.Context(), claims.UserID,
		service.CreateChatInput{
			Title: title,
			Model: model,
		},
		service.SendMessageInput{
			Content: req.Message,
			APIKey:  req.APIKey,
		},
	)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	response.OK(w, SimpleChatResponse{
		ChatID:   chat.ID.String(),
		Response: result.AssistantMessage.Content,
		Model:    result.AssistantMessage.Model,
		Tokens:   result.AssistantMessage.TokensUsed,
	})
}

// Explain handles POST /api/v1/explain.
// It creates a chat with an "explain" system prompt and returns the explanation.
func (h *FacadeHandler) Explain(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
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
	if req.APIKey == "" {
		response.BadRequest(w, "api_key is required")
		return
	}

	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	title := fmt.Sprintf("Explain: %s", truncate(req.Topic, 40))

	// Compose the explain prompt.
	prompt := fmt.Sprintf(
		"You are a clear, concise explainer. Explain the following topic in a way that is "+
			"easy to understand. Use examples where helpful. Be thorough but not verbose.\n\n"+
			"Topic: %s", req.Topic,
	)

	// Create a chat and send the explain prompt atomically.
	_, result, err := h.chatService.CreateChatAndSendFirstMessage(r.Context(), claims.UserID,
		service.CreateChatInput{
			Title: title,
			Model: model,
		},
		service.SendMessageInput{
			Content: prompt,
			APIKey:  req.APIKey,
			Role:    domain.RoleSystem, // Important: explicitly inject as system prompt
		},
	)
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

// AICouncil handles POST /api/v1/ai-council.
// It sends the same question to multiple AI models concurrently and returns all responses.
func (h *FacadeHandler) AICouncil(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
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
	if req.APIKey == "" {
		response.BadRequest(w, "api_key is required")
		return
	}

	models := req.Models
	if len(models) == 0 {
		models = append([]string(nil), defaultCouncilModels...)
	}

	seen := make(map[string]struct{}, len(models))
	filtered := make([]string, 0, len(models))
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" {
			response.BadRequest(w, "models must not contain empty values")
			return
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		filtered = append(filtered, m)
	}
	if len(filtered) > 5 {
		response.BadRequest(w, "at most 5 models are supported")
		return
	}
	models = filtered

	title := fmt.Sprintf("Council: %s", truncate(req.Question, 40))

	// Fan-out: query each model concurrently.
	var wg sync.WaitGroup
	opinions := make([]CouncilOpinion, len(models))

	for i, model := range models {
		wg.Add(1)
		go func(idx int, m string) {
			defer wg.Done()

			opinion := CouncilOpinion{Model: m}

			// Each model gets its own chat, performed atomically.
			_, result, err := h.chatService.CreateChatAndSendFirstMessage(r.Context(), claims.UserID,
				service.CreateChatInput{
					Title: fmt.Sprintf("%s [%s]", title, m),
					Model: m,
				},
				service.SendMessageInput{
					Content: req.Question,
					APIKey:  req.APIKey,
				},
			)
			if err != nil {
				opinion.Error = "model failed to respond"
				h.log.ErrorContext(r.Context(), "council: exchange failed",
					"model", m, "error", err)
				opinions[idx] = opinion
				return
			}

			opinion.Response = result.AssistantMessage.Content
			opinion.Tokens = result.AssistantMessage.TokensUsed
			opinions[idx] = opinion
		}(i, model)
	}

	wg.Wait()

	response.OK(w, AICouncilResponse{
		Question: req.Question,
		Opinions: opinions,
	})
}

// SessionSummary handles POST /api/v1/session/summary.
// It retrieves the message history of a chat and generates a summary.
func (h *FacadeHandler) SessionSummary(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		response.Unauthorized(w, "not authenticated")
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
	if req.APIKey == "" {
		response.BadRequest(w, "api_key is required")
		return
	}

	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	chatID, err := parseUUID(req.ChatID)
	if err != nil {
		response.BadRequest(w, "invalid chat_id format")
		return
	}

	// Get the chat and its message history.
	chat, messages, err := h.chatService.GetChat(r.Context(), claims.UserID, chatID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	if len(messages) == 0 {
		response.BadRequest(w, "chat has no messages to summarize")
		return
	}

	// Build a transcript from the message history.
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation concisely. ")
	sb.WriteString("Highlight key topics, decisions, and action items.\n\n")
	sb.WriteString("--- Conversation ---\n")
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}
	sb.WriteString("--- End ---\n\n")
	sb.WriteString("Provide a structured summary.")

	// Create a summary chat with the same model the original chat used (or override).
	summaryModel := model
	if chat.Model != "" && req.Model == "" {
		summaryModel = chat.Model
	}

	_, result, err := h.chatService.CreateChatAndSendFirstMessage(r.Context(), claims.UserID,
		service.CreateChatInput{
			Title: fmt.Sprintf("Summary: %s", truncate(chat.Title, 40)),
			Model: summaryModel,
		},
		service.SendMessageInput{
			Content: sb.String(),
			APIKey:  req.APIKey,
		},
	)
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

// ── Helpers ─────────────────────────────────────────────────────────────────

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

// truncate limits a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// parseUUID parses a string into a uuid.UUID.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
