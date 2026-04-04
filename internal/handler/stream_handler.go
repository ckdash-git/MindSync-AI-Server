package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/middleware"
	"github.com/ckdash-git/MindSync-AI-Server/internal/openrouter"
	"github.com/ckdash-git/MindSync-AI-Server/internal/service"
	"github.com/ckdash-git/MindSync-AI-Server/pkg/response"
	"github.com/go-chi/chi/v5"
)

// StreamHandler handles SSE streaming endpoints.
type StreamHandler struct {
	streamService *service.StreamService
	defaultModel  string
	log           *logger.Logger
}

// NewStreamHandler creates a new StreamHandler.
func NewStreamHandler(streamService *service.StreamService, defaultModel string, log *logger.Logger) *StreamHandler {
	return &StreamHandler{
		streamService: streamService,
		defaultModel:  defaultModel,
		log:           log,
	}
}

// RegisterRoutes registers streaming routes on the given router.
func (h *StreamHandler) RegisterRoutes(r chi.Router) {
	r.With(middleware.RequireAuth).Post("/chat/stream", h.Stream)
}

// StreamRequest is the incoming request body for streaming.
type StreamRequest struct {
	Model    string               `json:"model"`
	Messages []openrouter.Message `json:"messages"`
	APIKey   string               `json:"api_key,omitempty"` // BYOK
}

// Stream handles the SSE streaming endpoint.
func (h *StreamHandler) Stream(w http.ResponseWriter, r *http.Request) {
	// Parse request
	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}

	// Validate
	if len(req.Messages) == 0 {
		response.BadRequest(w, "messages are required")
		return
	}

	// Default model
	model := req.Model
	if model == "" {
		model = h.defaultModel
	}

	// API key (BYOK or from user's stored key)
	apiKey := req.APIKey
	if apiKey == "" {
		response.BadRequest(w, "API key is required (BYOK)")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Check if ResponseWriter supports Flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.log.ErrorContext(r.Context(), "response writer does not support flushing")
		response.InternalError(w)
		return
	}

	// Start streaming
	streamReq := service.StreamRequest{
		APIKey:   apiKey,
		Model:    model,
		Messages: req.Messages,
	}

	chunkCh, errCh, err := h.streamService.Stream(r.Context(), streamReq)
	if err != nil {
		h.log.ErrorContext(r.Context(), "stream start failed", "error", err)
		// Write SSE error event before closing
		fmt.Fprintf(w, "event: error\ndata: {\"message\":\"failed to start stream\"}\n\n")
		flusher.Flush()
		return
	}

	// Stream chunks to client
	for {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				// Stream ended
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
			// Client disconnected
			h.log.InfoContext(r.Context(), "client disconnected during stream")
			return
		}
	}
}
