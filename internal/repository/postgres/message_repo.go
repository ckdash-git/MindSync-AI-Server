package postgres

import (
	"context"
	"fmt"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/google/uuid"
)

// MessageRepo implements domain.MessageRepository with PostgreSQL.
type MessageRepo struct {
	db *DB
}

// NewMessageRepo creates a new MessageRepo.
func NewMessageRepo(db *DB) *MessageRepo {
	return &MessageRepo{db: db}
}

func (r *MessageRepo) Create(ctx context.Context, msg *domain.Message) error {
	query := `
		INSERT INTO messages (id, chat_id, role, content, model, tokens_used, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query,
		msg.ID, msg.ChatID, msg.Role, msg.Content,
		msg.Model, msg.TokensUsed, msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting message: %w", err)
	}
	return nil
}

func (r *MessageRepo) GetByChatID(ctx context.Context, chatID uuid.UUID, limit, offset int) ([]*domain.Message, error) {
	var messages []*domain.Message
	query := `
		SELECT id, chat_id, role, content, model, tokens_used, created_at
		FROM messages
		WHERE chat_id = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`
	if err := r.db.SelectContext(ctx, &messages, query, chatID, limit, offset); err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	return messages, nil
}

func (r *MessageRepo) DeleteByChatID(ctx context.Context, chatID uuid.UUID) error {
	query := `DELETE FROM messages WHERE chat_id = $1`
	_, err := r.db.ExecContext(ctx, query, chatID)
	if err != nil {
		return fmt.Errorf("deleting messages: %w", err)
	}
	return nil
}
