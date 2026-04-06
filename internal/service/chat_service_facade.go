package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/ckdash-git/MindSync-AI-Server/internal/openrouter"
	"github.com/google/uuid"
)

// FacadeChatInput contains parameters for the simplified chat endpoint.
type FacadeChatInput struct {
	ChatID  *uuid.UUID
	Message string
	Model   string
	APIKey  string
}

// truncateTitle limits a string to maxLen characters, appending "..." if truncated safely.
func truncateTitle(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// FacadeChat handles the logic for a simplified chat, reusing a chat if provided.
func (s *ChatService) FacadeChat(ctx context.Context, userID uuid.UUID, input FacadeChatInput) (*SendMessageResult, uuid.UUID, error) {
	var chatID uuid.UUID
	var result *SendMessageResult

	if input.ChatID != nil {
		chatID = *input.ChatID
		// Verify existence and ownership
		chat, err := s.chatRepo.GetByID(ctx, chatID)
		if err != nil {
			return nil, uuid.Nil, fmt.Errorf("invalid chat_id: %w", err)
		}
		if chat.UserID != userID {
			return nil, uuid.Nil, domain.ErrForbidden
		}

		result, err = s.SendMessage(ctx, userID, chatID, SendMessageInput{
			Content: input.Message,
			APIKey:  input.APIKey,
		})
		if err != nil {
			return nil, chatID, err
		}
	} else {
		// Auto-generate title
		title := truncateTitle(input.Message, 50)

		var err error
		var chat *domain.Chat
		chat, result, err = s.CreateChatAndSendFirstMessage(ctx, userID, CreateChatInput{
			Title: title,
			Model: input.Model,
		}, SendMessageInput{
			Content: input.Message,
			APIKey:  input.APIKey,
		})
		if err != nil {
			return nil, uuid.Nil, fmt.Errorf("creating chat and sending msg: %w", err)
		}
		chatID = chat.ID
	}

	return result, chatID, nil
}

// ExplainInput contains parameters for the explain endpoint.
type ExplainInput struct {
	Topic  string
	Model  string
	APIKey string
}

// Explain injects a system prompt and fetches an explanation for the given topic.
func (s *ChatService) Explain(ctx context.Context, userID uuid.UUID, input ExplainInput) (*SendMessageResult, error) {
	title := truncateTitle(fmt.Sprintf("Explain: %s", input.Topic), 50)

	prompt := fmt.Sprintf(
		"You are a clear, concise explainer. Explain the following topic in a way that is "+
			"easy to understand. Use examples where helpful. Be thorough but not verbose.\n\n"+
			"Topic: %s", input.Topic,
	)

	_, result, err := s.CreateChatAndSendFirstMessage(ctx, userID, CreateChatInput{
		Title: title,
		Model: input.Model,
	}, SendMessageInput{
		Content: prompt,
		APIKey:  input.APIKey,
		Role:    domain.RoleSystem,
	})
	if err != nil {
		return nil, fmt.Errorf("creating chat and sending msg: %w", err)
	}

	return result, nil
}

// AICouncilInput contains parameters for the ai-council endpoint.
type AICouncilInput struct {
	Question string
	Models   []string
	APIKey   string
}

// CouncilOpinion holds a single model's response within the council.
type CouncilOpinion struct {
	Model    string `json:"model"`
	Response string `json:"output"`
	Tokens   int    `json:"-"`
	Error    string `json:"error,omitempty"`
}

// AICouncilResult is the result of the AI council request.
type AICouncilResult struct {
	Opinions    []CouncilOpinion
	FinalAnswer string
}

// AICouncil fans out the question to multiple models concurrently and synthesizes a final answer.
func (s *ChatService) AICouncil(ctx context.Context, userID uuid.UUID, input AICouncilInput) (*AICouncilResult, error) {
	// Sanitize and deduplicate models
	models := input.Models
	if len(models) == 0 {
		models = []string{
			"openai/gpt-4o",
			"anthropic/claude-3.5-sonnet",
			"google/gemini-pro",
		}
	}

	seen := make(map[string]struct{}, len(models))
	filtered := make([]string, 0, len(models))
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" {
			return nil, domain.NewValidationError("models", "models must not contain empty values")
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		filtered = append(filtered, m)
	}
	if len(filtered) > 5 {
		return nil, domain.NewValidationError("models", "at most 5 models are supported")
	}
	input.Models = filtered

	// Enforce an overall timeout of structured 2 minutes
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	opinions := make([]CouncilOpinion, len(input.Models))

	title := truncateTitle(fmt.Sprintf("Council: %s", input.Question), 50)

	for i, m := range input.Models {
		wg.Add(1)
		go func(idx int, model string) {
			defer wg.Done()
			opinion := CouncilOpinion{Model: model}

			_, result, err := s.CreateChatAndSendFirstMessage(ctx, userID, CreateChatInput{
				Title: fmt.Sprintf("%s [%s]", title, model),
				Model: model,
			}, SendMessageInput{
				Content: input.Question,
				APIKey:  input.APIKey,
			})
			if err != nil {
				opinion.Error = "model failed to respond or chat creation failed"
				opinions[idx] = opinion
				s.log.ErrorContext(ctx, "council step failed", "model", model, "error", err)
				return
			}

			opinion.Response = result.AssistantMessage.Content
			opinion.Tokens = result.AssistantMessage.TokensUsed
			opinions[idx] = opinion
		}(i, m)
	}

	wg.Wait()

	// Synthesize final answer combining opinions
	var validOpinions []CouncilOpinion
	for _, o := range opinions {
		if o.Error == "" {
			validOpinions = append(validOpinions, o)
		}
	}

	if len(validOpinions) == 0 {
		// All models failed
		return &AICouncilResult{Opinions: opinions, FinalAnswer: "All models failed to provide an answer."}, nil
	}

	// Request final answer using the first model that succeeded in council fan-out
	synthesisModel := validOpinions[0].Model

	synthChat, err := s.CreateChat(ctx, userID, CreateChatInput{
		Title: fmt.Sprintf("Final: %s", title),
		Model: synthesisModel,
	})
	if err != nil {
		s.log.ErrorContext(ctx, "failed to create synthesis chat", "error", err)
		return &AICouncilResult{Opinions: opinions, FinalAnswer: "Failed to synthesize final answer."}, nil
	}

	var sb strings.Builder
	sb.WriteString("You are a synthesizer. Review the following answers to a question and provide a final, well-reasoned consensus or synthesis.\n\n")
	sb.WriteString(fmt.Sprintf("Question: %s\n\n", input.Question))
	for _, o := range validOpinions {
		sb.WriteString(fmt.Sprintf("--- Idea from model %s ---\n%s\n\n", o.Model, o.Response))
	}

	res, err := s.SendMessage(ctx, userID, synthChat.ID, SendMessageInput{
		Content: sb.String(),
		APIKey:  input.APIKey,
	})
	
	finalAnswer := ""
	if err != nil {
		finalAnswer = "Synthesis failed to respond."
	} else {
		finalAnswer = res.AssistantMessage.Content
	}

	return &AICouncilResult{
		Opinions:    opinions,
		FinalAnswer: finalAnswer,
	}, nil
}

// SessionSummaryInput contains parameters for the session summary endpoint.
type SessionSummaryInput struct {
	ChatID uuid.UUID
	Model  string
	APIKey string
}

// SessionSummary delegates rendering a brief summary form the history to the ChatService.
func (s *ChatService) SessionSummary(ctx context.Context, userID uuid.UUID, input SessionSummaryInput) (*SendMessageResult, error) {
	chat, messages, err := s.GetChat(ctx, userID, input.ChatID)
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, domain.NewValidationError("chat_id", "chat has no messages to summarize")
	}

	var sb strings.Builder
	sb.WriteString("Summarize the following conversation concisely. ")
	sb.WriteString("Highlight key topics, decisions, and action items.\n\n")
	sb.WriteString("--- Conversation ---\n")
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}
	sb.WriteString("--- End ---\n\n")
	sb.WriteString("Provide a structured summary.")

	summaryModel := input.Model
	if summaryModel == "" {
		summaryModel = chat.Model
	}

	_, result, err := s.CreateChatAndSendFirstMessage(ctx, userID, CreateChatInput{
		Title: truncateTitle(fmt.Sprintf("Summary: %s", chat.Title), 50),
		Model: summaryModel,
	}, SendMessageInput{
		Content: sb.String(),
		APIKey:  input.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("creating summary chat and msg: %w", err)
	}

	return result, nil
}

// FacadeStreamInput contains parameters for the streaming endpoint.
type FacadeStreamInput struct {
	ChatID  *uuid.UUID
	Message string
	Model   string
	APIKey  string
}

// FacadeStream handles the streaming lifecycle: DB creation, intercepting stream chunks, 
// immediately passing them to the client, forming the final message, and saving it to the DB asynchronously.
func (s *ChatService) FacadeStream(ctx context.Context, userID uuid.UUID, input FacadeStreamInput, streamSvc *StreamService) (<-chan StreamChunk, <-chan error, string, error) {
	if streamSvc == nil {
		return nil, nil, "", fmt.Errorf("stream service is required")
	}

	var chatID uuid.UUID
	var chat *domain.Chat

	if input.ChatID != nil {
		chatID = *input.ChatID
		c, err := s.chatRepo.GetByID(ctx, chatID)
		if err != nil {
			return nil, nil, "", fmt.Errorf("invalid chat_id: %w", err)
		}
		if c.UserID != userID {
			return nil, nil, "", domain.ErrForbidden
		}
		chat = c
	} else {
		title := truncateTitle(input.Message, 50)
		c, err := s.CreateChat(ctx, userID, CreateChatInput{
			Title: title,
			Model: input.Model,
		})
		if err != nil {
			return nil, nil, "", fmt.Errorf("creating chat: %w", err)
		}
		chatID = c.ID
		chat = c
	}

	// Save user message first.
	userMsg := &domain.Message{
		ID:        uuid.New(),
		ChatID:    chatID,
		Role:      domain.RoleUser,
		Content:   input.Message,
		CreatedAt: time.Now(),
	}
	if err := s.messageRepo.Create(ctx, userMsg); err != nil {
		return nil, nil, "", fmt.Errorf("saving user msg: %w", err)
	}

	// Gather history
	history, err := s.messageRepo.GetByChatID(ctx, chatID, 50, 0)
	if err != nil {
		return nil, nil, "", fmt.Errorf("getting history: %w", err)
	}

	var orMessages []openrouter.Message
	for _, msg := range history {
		// Note: The privacy proc processes this in StreamService later, 
		// but openrouter expects []openrouter.Message at StreamRequest boundary.
		orMessages = append(orMessages, openrouter.Message{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	// Since streamSvc requires raw string slice in messages, we send it off:
	req := StreamRequest{
		Model:    chat.Model,
		Messages: orMessages,
		APIKey:   input.APIKey,
	}

	chunkCh, errCh, err := streamSvc.Stream(ctx, req)
	if err != nil {
		return nil, nil, "", err
	}

	outChunk := make(chan StreamChunk, 64)
	outErr := make(chan error, 1)

	// Intercept chunks asynchronously
	go func() {
		defer close(outChunk)
		defer close(outErr)

		var buffer strings.Builder
		finalModel := chat.Model

		for chunkCh != nil || errCh != nil {
			select {
			case chunk, ok := <-chunkCh:
				if !ok {
					chunkCh = nil
					continue
				}

				if chunk.Model != "" {
					finalModel = chunk.Model
				}
				buffer.WriteString(chunk.Content)

				// Pass through
				select {
				case outChunk <- chunk:
				case <-ctx.Done():
				}

			case err, ok := <-errCh:
				if !ok {
					errCh = nil
					continue
				}
				if err != nil {
					outErr <- err
					s.log.ErrorContext(ctx, "facade stream interrupted by err", "error", err)
				}
				
			case <-ctx.Done():
				outErr <- ctx.Err()
				chunkCh = nil
				errCh = nil // Force exit loop
			}
		}

		if buffer.Len() > 0 {
			assistantMsg := &domain.Message{
				ID:        uuid.New(),
				ChatID:    chatID,
				Role:      domain.RoleAssistant,
				Content:   buffer.String(),
				Model:     finalModel,
				CreatedAt: time.Now(),
			}

			persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := s.messageRepo.Create(persistCtx, assistantMsg); err != nil {
				s.log.ErrorContext(persistCtx, "failed to persist assistant message", "chat_id", chatID, "message_id", assistantMsg.ID, "error", err)
			}
			cancel()
		}
	}()

	return outChunk, outErr, chatID.String(), nil
}
