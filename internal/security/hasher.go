package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// bcryptCost is the computational cost for bcrypt hashing.
	// 12 is a good balance of security vs. performance.
	bcryptCost = 12
)

// HashPassword creates a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword verifies a password against a bcrypt hash.
// Returns nil on match, error otherwise.
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// HashRefreshToken creates a SHA-256 hash of a refresh token for storage.
// We don't use bcrypt here because we need to look up tokens by hash.
func HashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
