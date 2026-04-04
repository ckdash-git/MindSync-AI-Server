package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/openrouter"
	"github.com/ckdash-git/MindSync-AI-Server/internal/privacy"
	"github.com/google/uuid"
)

// ChatService handles chat business logic.
type ChatService struct {
	chatRepo    domain.ChatRepository
	messageRepo domain.MessageRepository
	orClient    *openrouter.Client
	privacyProc *privacy.Processor
	log         *logger.Logger
}

// NewChatService creates a new ChatService.
func NewChatService(
	chatRepo domain.ChatRepository,
	messageRepo domain.MessageRepository,
	orClient *openrouter.Client,
	privacyProc *privacy.Processor,
	log *logger.Logger,
) *ChatService {
	return &ChatService{
		chatRepo:    chatRepo,
		messageRepo: messageRepo,
		orClient:    orClient,
		privacyProc: privacyProc,
		log:         log,
	}
}

// CreateChatInput holds data for creating a new chat.
type CreateChatInput struct {
	Title string `json:"title"`
	Model string `json:"model"`
}

// SendMessageInput holds data for sending a message.
type SendMessageInput struct {
	Content string `json:"content"`
	APIKey  string `json:"-"` // BYOK key, never serialized
}

// SendMessageResult holds the result of sending a message.
type SendMessageResult struct {
	UserMessage      *domain.Message `json:"user_message"`
	AssistantMessage *domain.Message `json:"assistant_message"`
}

// CreateChat creates a new chat conversation.
func (s *ChatService) CreateChat(ctx context.Context, userID uuid.UUID, input CreateChatInput) (*domain.Chat, error) {
	if input.Title == "" {
		return nil, domain.NewValidationError("title", "title is required")
	}

	chat := &domain.Chat{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     input.Title,
		Model:     input.Model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.chatRepo.Create(ctx, chat); err != nil {
		s.log.ErrorContext(ctx, "creating chat", "error", err)
		return nil, fmt.Errorf("creating chat: %w", err)
	}

	s.log.InfoContext(ctx, "chat created", "chat_id", chat.ID)
	return chat, nil
}

// GetChat retrieves a chat by ID, verifying ownership.
func (s *ChatService) GetChat(ctx context.Context, userID, chatID uuid.UUID) (*domain.Chat, []*domain.Message, error) {
	chat, err := s.chatRepo.GetByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, fmt.Errorf("getting chat: %w", err)
	}

	if chat.UserID != userID {
		return nil, nil, domain.ErrForbidden
	}

	messages, err := s.messageRepo.GetByChatID(ctx, chatID, 100, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("getting messages: %w", err)
	}

	return chat, messages, nil
}

// ListChats returns paginated chats for a user.
func (s *ChatService) ListChats(ctx context.Context, userID uuid.UUID, page, perPage int) ([]*domain.Chat, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 20
	}

	offset := (page - 1) * perPage
	chats, total, err := s.chatRepo.ListByUser(ctx, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing chats: %w", err)
	}

	return chats, total, nil
}

// DeleteChat deletes a chat and its messages, verifying ownership.
func (s *ChatService) DeleteChat(ctx context.Context, userID, chatID uuid.UUID) error {
	chat, err := s.chatRepo.GetByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("getting chat: %w", err)
	}

	if chat.UserID != userID {
		return domain.ErrForbidden
	}

	// Delete messages first
	if err := s.messageRepo.DeleteByChatID(ctx, chatID); err != nil {
		return fmt.Errorf("deleting messages: %w", err)
	}

	if err := s.chatRepo.Delete(ctx, chatID); err != nil {
		return fmt.Errorf("deleting chat: %w", err)
	}

	s.log.InfoContext(ctx, "chat deleted", "chat_id", chatID)
	return nil
}

// SendMessage sends a non-streaming message through the privacy proxy.
func (s *ChatService) SendMessage(ctx context.Context, userID, chatID uuid.UUID, input SendMessageInput) (*SendMessageResult, error) {
	if input.Content == "" {
		return nil, domain.NewValidationError("content", "message content is required")
	}
	if input.APIKey == "" {
		return nil, domain.NewValidationError("api_key", "API key is required")
	}

	// Verify chat ownership
	chat, err := s.chatRepo.GetByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("getting chat: %w", err)
	}

	if chat.UserID != userID {
		return nil, domain.ErrForbidden
	}

	// Save user message
	userMsg := &domain.Message{
		ID:        uuid.New(),
		ChatID:    chatID,
		Role:      domain.RoleUser,
		Content:   input.Content,
		CreatedAt: time.Now(),
	}

	if err := s.messageRepo.Create(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("saving user message: %w", err)
	}

	// Get conversation history for context
	history, err := s.messageRepo.GetByChatID(ctx, chatID, 50, 0)
	if err != nil {
		return nil, fmt.Errorf("getting history: %w", err)
	}

	// Privacy proxy: sanitize messages
	mapping := privacy.NewTokenMapping()
	privCtx := privacy.WithTokenMapping(ctx, mapping)

	var orMessages []openrouter.Message
	for _, msg := range history {
		sanitized, _ := s.privacyProc.Sanitize(privCtx, msg.Content)
		orMessages = append(orMessages, openrouter.Message{
			Role:    string(msg.Role),
			Content: sanitized,
		})
	}

	// Send to OpenRouter
	orReq := &openrouter.ChatRequest{
		Model:    chat.Model,
		Messages: orMessages,
	}

	orResp, err := s.orClient.SendChat(ctx, input.APIKey, orReq)
	if err != nil {
		return nil, fmt.Errorf("calling OpenRouter: %w", err)
	}

	// Rehydrate response
	assistantContent := ""
	tokensUsed := 0
	if len(orResp.Choices) > 0 {
		rehydrator := privacy.NewStreamRehydrator(mapping)
		assistantContent = rehydrator.Process(orResp.Choices[0].Message.Content)
		assistantContent += rehydrator.Flush()
		tokensUsed = orResp.Usage.TotalTokens
	}

	// Save assistant message
	assistantMsg := &domain.Message{
		ID:         uuid.New(),
		ChatID:     chatID,
		Role:       domain.RoleAssistant,
		Content:    assistantContent,
		Model:      orResp.Model,
		TokensUsed: tokensUsed,
		CreatedAt:  time.Now(),
	}

	if err := s.messageRepo.Create(ctx, assistantMsg); err != nil {
		return nil, fmt.Errorf("saving assistant message: %w", err)
	}

	return &SendMessageResult{
		UserMessage:      userMsg,
		AssistantMessage: assistantMsg,
	}, nil
}
