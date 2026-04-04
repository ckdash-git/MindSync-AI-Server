package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Session represents an authentication session with refresh token.
type Session struct {
	ID           uuid.UUID `json:"id" db:"id"`
	UserID       uuid.UUID `json:"user_id" db:"user_id"`
	RefreshToken string    `json:"-" db:"refresh_token"` // hashed before storage
	UserAgent    string    `json:"-" db:"user_agent"`
	IPAddress    string    `json:"-" db:"ip_address"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// IsExpired checks if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// SessionRepository defines the data access interface for sessions.
type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	GetByRefreshToken(ctx context.Context, tokenHash string) (*Session, error)
	DeleteByID(ctx context.Context, id uuid.UUID) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}
