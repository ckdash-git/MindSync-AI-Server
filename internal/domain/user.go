package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User represents a registered user.
type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`                 // encrypted at rest
	PasswordHash string    `json:"-" db:"password_hash"`             // never exposed in JSON
	DisplayName  string    `json:"display_name" db:"display_name"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// UserRepository defines the data access interface for users.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uuid.UUID) error
}
