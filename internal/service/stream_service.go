package service

import (
	"context"
	"fmt"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/openrouter"
	"github.com/ckdash-git/MindSync-AI-Server/internal/privacy"
)

// StreamService orchestrates streaming chat with privacy proxy.
type StreamService struct {
	orClient    *openrouter.Client
	privacyProc *privacy.Processor
	log         *logger.Logger
}

// NewStreamService creates a new StreamService.
func NewStreamService(
	orClient *openrouter.Client,
	privacyProc *privacy.Processor,
	log *logger.Logger,
) *StreamService {
	return &StreamService{
		orClient:    orClient,
		privacyProc: privacyProc,
		log:         log,
	}
}

// StreamRequest holds the data for a streaming chat request.
type StreamRequest struct {
	APIKey   string               `json:"-"` // Never serialized
	Model    string               `json:"model"`
	Messages []openrouter.Message `json:"messages"`
}

// StreamChunk represents a single chunk sent to the client.
type StreamChunk struct {
	Content      string `json:"content"`
	Model        string `json:"model,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
	Done         bool   `json:"done"`
}

// Stream sends a streaming request through the privacy proxy and returns a channel of chunks.
func (s *StreamService) Stream(ctx context.Context, req StreamRequest) (<-chan StreamChunk, <-chan error, error) {
	// === Privacy Proxy: Sanitize outbound messages ===
	mapping := privacy.NewTokenMapping()
	ctx = privacy.WithTokenMapping(ctx, mapping)

	sanitizedMessages := make([]openrouter.Message, len(req.Messages))
	for i, msg := range req.Messages {
		sanitizedContent, err := s.privacyProc.Sanitize(ctx, msg.Content)
		if err != nil {
			s.log.ErrorContext(ctx, "privacy sanitization failed", "error", err)
			return nil, nil, fmt.Errorf("sanitizing message: %w", err)
		}
		sanitizedMessages[i] = openrouter.Message{
			Role:    msg.Role,
			Content: sanitizedContent,
		}
	}

	// === Send to OpenRouter ===
	orReq := &openrouter.ChatRequest{
		Model:    req.Model,
		Messages: sanitizedMessages,
	}

	eventCh, upstreamErrCh, err := s.orClient.StreamChat(ctx, req.APIKey, orReq)
	if err != nil {
		return nil, nil, fmt.Errorf("starting stream: %w", err)
	}

	// === Rehydrate response and forward to client ===
	chunkCh := make(chan StreamChunk, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		rehydrator := privacy.NewStreamRehydrator(mapping)

		for event := range eventCh {
			for _, choice := range event.Choices {
				if choice.Delta == nil || choice.Delta.Content == "" {
					// Check for finish_reason without content
					if choice.FinishReason != "" {
						// Flush remaining buffer
						remaining := rehydrator.Flush()
						if remaining != "" {
							select {
							case chunkCh <- StreamChunk{Content: remaining, Model: event.Model}:
							case <-ctx.Done():
								errCh <- ctx.Err()
								return
							}
						}

						select {
						case chunkCh <- StreamChunk{
							FinishReason: choice.FinishReason,
							Model:        event.Model,
							Done:         true,
						}:
						case <-ctx.Done():
							errCh <- ctx.Err()
							return
						}
					}
					continue
				}

				// Rehydrate token content through rolling buffer
				rehydrated := rehydrator.Process(choice.Delta.Content)
				if rehydrated == "" {
					continue // Content buffered, waiting for complete token
				}

				select {
				case chunkCh <- StreamChunk{
					Content: rehydrated,
					Model:   event.Model,
				}:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}
			}
		}

		// Flush any remaining buffered content
		if remaining := rehydrator.Flush(); remaining != "" {
			select {
			case chunkCh <- StreamChunk{Content: remaining}:
			case <-ctx.Done():
			}
		}

		// Forward upstream errors
		select {
		case err, ok := <-upstreamErrCh:
			if ok && err != nil {
				errCh <- err
			}
		default:
		}
	}()

	return chunkCh, errCh, nil
}
