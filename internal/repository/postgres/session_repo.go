package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/google/uuid"
)

// SessionRepo implements domain.SessionRepository with PostgreSQL.
type SessionRepo struct {
	db *DB
}

// NewSessionRepo creates a new SessionRepo.
func NewSessionRepo(db *DB) *SessionRepo {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) Create(ctx context.Context, session *domain.Session) error {
	query := `
		INSERT INTO sessions (id, user_id, refresh_token, user_agent, ip_address, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query,
		session.ID, session.UserID, session.RefreshToken,
		session.UserAgent, session.IPAddress,
		session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

func (r *SessionRepo) GetByRefreshToken(ctx context.Context, tokenHash string) (*domain.Session, error) {
	var session domain.Session
	query := `SELECT id, user_id, refresh_token, user_agent, ip_address, expires_at, created_at FROM sessions WHERE refresh_token = $1`
	err := r.db.GetContext(ctx, &session, query, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("getting session: %w", err)
	}
	return &session, nil
}

func (r *SessionRepo) DeleteByID(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM sessions WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

func (r *SessionRepo) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM sessions WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("deleting user sessions: %w", err)
	}
	return nil
}

func (r *SessionRepo) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM sessions WHERE expires_at < NOW()`
	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	return result.RowsAffected()
}
