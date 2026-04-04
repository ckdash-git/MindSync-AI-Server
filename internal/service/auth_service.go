package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"time"
	"unicode/utf8"

	"github.com/ckdash-git/MindSync-AI-Server/internal/domain"
	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
	"github.com/ckdash-git/MindSync-AI-Server/internal/security"
	"github.com/google/uuid"
)

// AuthService handles authentication business logic.
type AuthService struct {
	userRepo    domain.UserRepository
	sessionRepo domain.SessionRepository
	jwt         *security.JWTManager
	log         *logger.Logger
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	userRepo domain.UserRepository,
	sessionRepo domain.SessionRepository,
	jwt *security.JWTManager,
	log *logger.Logger,
) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		jwt:         jwt,
		log:         log,
	}
}

// RegisterInput holds the data needed to register a user.
type RegisterInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// LoginInput holds the data needed to log in.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResult is returned after successful authentication.
type AuthResult struct {
	User      *domain.User       `json:"user"`
	TokenPair *security.TokenPair `json:"tokens"`
}

// Register creates a new user account.
func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*AuthResult, error) {
	// Validate input
	if err := validateRegisterInput(input); err != nil {
		return nil, err
	}

	// Check if user already exists
	existing, err := s.userRepo.GetByEmail(ctx, input.Email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		s.log.ErrorContext(ctx, "checking existing user", "error", err)
		return nil, fmt.Errorf("checking existing user: %w", err)
	}
	if existing != nil {
		return nil, domain.ErrUserAlreadyExists
	}

	// Hash password
	hash, err := security.HashPassword(input.Password)
	if err != nil {
		s.log.ErrorContext(ctx, "hashing password", "error", err)
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	// Create user
	user := &domain.User{
		ID:           uuid.New(),
		Email:        input.Email,
		PasswordHash: hash,
		DisplayName:  input.DisplayName,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		s.log.ErrorContext(ctx, "creating user", "error", err)
		return nil, fmt.Errorf("creating user: %w", err)
	}

	// Generate tokens
	tokenPair, err := s.jwt.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		s.log.ErrorContext(ctx, "generating tokens", "error", err)
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	// Create session
	session := &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: security.HashRefreshToken(tokenPair.RefreshToken),
		ExpiresAt:    time.Now().Add(s.jwt.RefreshExpiry()),
		CreatedAt:    time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		s.log.ErrorContext(ctx, "creating session", "error", err)
		return nil, fmt.Errorf("creating session: %w", err)
	}

	s.log.InfoContext(ctx, "user registered", "user_id", user.ID)

	return &AuthResult{
		User:      user,
		TokenPair: tokenPair,
	}, nil
}

// Login authenticates a user and returns tokens.
func (s *AuthService) Login(ctx context.Context, input LoginInput) (*AuthResult, error) {
	if input.Email == "" || input.Password == "" {
		return nil, domain.NewValidationError("credentials", "email and password are required")
	}

	// Find user
	user, err := s.userRepo.GetByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		s.log.ErrorContext(ctx, "finding user", "error", err)
		return nil, fmt.Errorf("finding user: %w", err)
	}

	// Verify password
	if err := security.CheckPassword(input.Password, user.PasswordHash); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	// Generate tokens
	tokenPair, err := s.jwt.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		s.log.ErrorContext(ctx, "generating tokens", "error", err)
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	// Create session
	session := &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: security.HashRefreshToken(tokenPair.RefreshToken),
		ExpiresAt:    time.Now().Add(s.jwt.RefreshExpiry()),
		CreatedAt:    time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		s.log.ErrorContext(ctx, "creating session", "error", err)
		return nil, fmt.Errorf("creating session: %w", err)
	}

	s.log.InfoContext(ctx, "user logged in", "user_id", user.ID)

	return &AuthResult{
		User:      user,
		TokenPair: tokenPair,
	}, nil
}

// RefreshTokens validates a refresh token and issues a new token pair.
func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*AuthResult, error) {
	if refreshToken == "" {
		return nil, domain.NewValidationError("refresh_token", "refresh token is required")
	}

	tokenHash := security.HashRefreshToken(refreshToken)

	session, err := s.sessionRepo.GetByRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrInvalidToken
		}
		s.log.ErrorContext(ctx, "finding session", "error", err)
		return nil, fmt.Errorf("finding session: %w", err)
	}

	if session.IsExpired() {
		// Clean up expired session
		_ = s.sessionRepo.DeleteByID(ctx, session.ID)
		return nil, domain.ErrSessionExpired
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		s.log.ErrorContext(ctx, "finding user for refresh", "error", err)
		return nil, fmt.Errorf("finding user: %w", err)
	}

	// Rotate: delete old session, create new one
	_ = s.sessionRepo.DeleteByID(ctx, session.ID)

	// Generate new tokens
	tokenPair, err := s.jwt.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		s.log.ErrorContext(ctx, "generating tokens", "error", err)
		return nil, fmt.Errorf("generating tokens: %w", err)
	}

	newSession := &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: security.HashRefreshToken(tokenPair.RefreshToken),
		ExpiresAt:    time.Now().Add(s.jwt.RefreshExpiry()),
		CreatedAt:    time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, newSession); err != nil {
		s.log.ErrorContext(ctx, "creating new session", "error", err)
		return nil, fmt.Errorf("creating session: %w", err)
	}

	s.log.InfoContext(ctx, "tokens refreshed", "user_id", user.ID)

	return &AuthResult{
		User:      user,
		TokenPair: tokenPair,
	}, nil
}

// Logout invalidates all sessions for the user.
func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID) error {
	if err := s.sessionRepo.DeleteByUserID(ctx, userID); err != nil {
		s.log.ErrorContext(ctx, "deleting sessions", "error", err)
		return fmt.Errorf("deleting sessions: %w", err)
	}

	s.log.InfoContext(ctx, "user logged out", "user_id", userID)
	return nil
}

func validateRegisterInput(input RegisterInput) error {
	if input.Email == "" {
		return domain.NewValidationError("email", "email is required")
	}
	if _, err := mail.ParseAddress(input.Email); err != nil {
		return domain.NewValidationError("email", "invalid email format")
	}
	if input.Password == "" {
		return domain.NewValidationError("password", "password is required")
	}
	if utf8.RuneCountInString(input.Password) < 8 {
		return domain.NewValidationError("password", "password must be at least 8 characters")
	}
	if input.DisplayName == "" {
		return domain.NewValidationError("display_name", "display name is required")
	}
	return nil
}
