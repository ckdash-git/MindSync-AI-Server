package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Chat represents a conversation with an AI model.
type Chat struct {
	ID        uuid.UUID `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Title     string    `json:"title" db:"title"`
	Model     string    `json:"model" db:"model"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// ChatRepository defines the data access interface for chats.
type ChatRepository interface {
	Create(ctx context.Context, chat *Chat) error
	GetByID(ctx context.Context, id uuid.UUID) (*Chat, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Chat, int, error)
	Update(ctx context.Context, chat *Chat) error
	Delete(ctx context.Context, id uuid.UUID) error
}
