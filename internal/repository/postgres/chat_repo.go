package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/google/uuid"
)

// ChatRepo implements domain.ChatRepository with PostgreSQL.
type ChatRepo struct {
	db *DB
}

// NewChatRepo creates a new ChatRepo.
func NewChatRepo(db *DB) *ChatRepo {
	return &ChatRepo{db: db}
}

func (r *ChatRepo) Create(ctx context.Context, chat *domain.Chat) error {
	query := `
		INSERT INTO chats (id, user_id, title, model, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query,
		chat.ID, chat.UserID, chat.Title, chat.Model,
		chat.CreatedAt, chat.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting chat: %w", err)
	}
	return nil
}

func (r *ChatRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Chat, error) {
	var chat domain.Chat
	query := `SELECT id, user_id, title, model, created_at, updated_at FROM chats WHERE id = $1`
	err := r.db.GetContext(ctx, &chat, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("getting chat: %w", err)
	}
	return &chat, nil
}

func (r *ChatRepo) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Chat, int, error) {
	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM chats WHERE user_id = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, userID); err != nil {
		return nil, 0, fmt.Errorf("counting chats: %w", err)
	}

	// Get paginated results
	var chats []*domain.Chat
	query := `
		SELECT id, user_id, title, model, created_at, updated_at
		FROM chats
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`
	if err := r.db.SelectContext(ctx, &chats, query, userID, limit, offset); err != nil {
		return nil, 0, fmt.Errorf("listing chats: %w", err)
	}

	return chats, total, nil
}

func (r *ChatRepo) Update(ctx context.Context, chat *domain.Chat) error {
	query := `UPDATE chats SET title = $1, model = $2, updated_at = $3 WHERE id = $4`
	_, err := r.db.ExecContext(ctx, query, chat.Title, chat.Model, chat.UpdatedAt, chat.ID)
	if err != nil {
		return fmt.Errorf("updating chat: %w", err)
	}
	return nil
}

func (r *ChatRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM chats WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting chat: %w", err)
	}
	return nil
}
