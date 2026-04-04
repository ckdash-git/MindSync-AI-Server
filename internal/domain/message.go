package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MessageRole defines the role of a message sender.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// Message represents a single message in a chat conversation.
type Message struct {
	ID         uuid.UUID   `json:"id" db:"id"`
	ChatID     uuid.UUID   `json:"chat_id" db:"chat_id"`
	Role       MessageRole `json:"role" db:"role"`
	Content    string      `json:"content" db:"content"`       // encrypted at rest
	Model      string      `json:"model,omitempty" db:"model"` // which AI model responded
	TokensUsed int         `json:"tokens_used,omitempty" db:"tokens_used"`
	CreatedAt  time.Time   `json:"created_at" db:"created_at"`
}

// MessageRepository defines the data access interface for messages.
type MessageRepository interface {
	Create(ctx context.Context, msg *Message) error
	GetByChatID(ctx context.Context, chatID uuid.UUID, limit, offset int) ([]*Message, error)
	DeleteByChatID(ctx context.Context, chatID uuid.UUID) error
}
