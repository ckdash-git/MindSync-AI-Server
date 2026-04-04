package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Encryptor provides AES-256-GCM encryption with versioned key rotation.
//
// Encrypted values are stored as: "v1:base64encodedciphertext"
// The version prefix allows decryption with the correct key even after rotation.
type Encryptor struct {
	mu         sync.RWMutex
	keys       map[string][]byte // version -> raw key bytes
	currentVer string            // version to use for new encryptions
}

// NewEncryptor creates a new Encryptor with versioned keys.
// keys: map of version (e.g., "v1") to hex-encoded 32-byte key.
// currentVer: which version to use for new encryptions.
func NewEncryptor(keys map[string]string, currentVer string) (*Encryptor, error) {
	if _, ok := keys[currentVer]; !ok {
		return nil, fmt.Errorf("current key version %q not found in provided keys", currentVer)
	}

	decodedKeys := make(map[string][]byte, len(keys))
	for ver, hexKey := range keys {
		key, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, fmt.Errorf("decoding key %s: %w", ver, err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("key %s must be 32 bytes (got %d)", ver, len(key))
		}
		decodedKeys[ver] = key
	}

	return &Encryptor{
		keys:       decodedKeys,
		currentVer: currentVer,
	}, nil
}

// Encrypt encrypts plaintext using the current key version.
// Returns: "version:base64ciphertext"
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	e.mu.RLock()
	key := e.keys[e.currentVer]
	ver := e.currentVer
	e.mu.RUnlock()

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return fmt.Sprintf("%s:%s", ver, encoded), nil
}

// Decrypt decrypts a versioned ciphertext string.
// Expects format: "version:base64ciphertext"
func (e *Encryptor) Decrypt(encrypted string) (string, error) {
	parts := strings.SplitN(encrypted, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid encrypted format: missing version prefix")
	}

	ver := parts[0]
	encoded := parts[1]

	e.mu.RLock()
	key, ok := e.keys[ver]
	e.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unknown key version: %s", ver)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting: %w", err)
	}

	return string(plaintext), nil
}

// ReEncrypt decrypts with the old key and re-encrypts with the current key.
// Useful for key rotation migration.
func (e *Encryptor) ReEncrypt(encrypted string) (string, error) {
	plaintext, err := e.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("re-encrypt decrypt: %w", err)
	}
	return e.Encrypt(plaintext)
}

// CurrentVersion returns the current encryption key version.
func (e *Encryptor) CurrentVersion() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentVer
}

// NeedsReEncrypt checks if the encrypted value uses an old key version.
func (e *Encryptor) NeedsReEncrypt(encrypted string) bool {
	parts := strings.SplitN(encrypted, ":", 2)
	if len(parts) != 2 {
		return true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return parts[0] != e.currentVer
}
