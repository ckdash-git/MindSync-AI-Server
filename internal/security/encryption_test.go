package security

import (
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// 32-byte key in hex (64 hex chars)
	keys := map[string]string{
		"v1": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	enc, err := NewEncryptor(keys, "v1")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "sensitive-data-like-api-key-or-email"
	encrypted, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Verify format: "v1:base64..."
	if !strings.HasPrefix(encrypted, "v1:") {
		t.Errorf("encrypted should start with 'v1:', got %q", encrypted)
	}

	// Verify decryption
	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("Decrypt = %q, want %q", decrypted, plaintext)
	}
}

func TestKeyRotation(t *testing.T) {
	keys := map[string]string{
		"v1": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"v2": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	}

	// Start with v1
	enc, err := NewEncryptor(keys, "v1")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "my-secret-api-key"
	encryptedV1, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt v1: %v", err)
	}

	if !strings.HasPrefix(encryptedV1, "v1:") {
		t.Errorf("should be v1, got %q", encryptedV1)
	}

	// Simulate rotation: create new encryptor with v2 as current
	enc2, err := NewEncryptor(keys, "v2")
	if err != nil {
		t.Fatalf("NewEncryptor v2: %v", err)
	}

	// Can still decrypt v1 data
	decrypted, err := enc2.Decrypt(encryptedV1)
	if err != nil {
		t.Fatalf("Decrypt v1 with v2 encryptor: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}

	// New encryptions use v2
	encryptedV2, err := enc2.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt v2: %v", err)
	}
	if !strings.HasPrefix(encryptedV2, "v2:") {
		t.Errorf("should be v2, got %q", encryptedV2)
	}

	// NeedsReEncrypt
	if !enc2.NeedsReEncrypt(encryptedV1) {
		t.Error("v1 data should need re-encryption")
	}
	if enc2.NeedsReEncrypt(encryptedV2) {
		t.Error("v2 data should NOT need re-encryption")
	}

	// ReEncrypt
	reEncrypted, err := enc2.ReEncrypt(encryptedV1)
	if err != nil {
		t.Fatalf("ReEncrypt: %v", err)
	}
	if !strings.HasPrefix(reEncrypted, "v2:") {
		t.Errorf("re-encrypted should be v2, got %q", reEncrypted)
	}

	decrypted2, err := enc2.Decrypt(reEncrypted)
	if err != nil {
		t.Fatalf("Decrypt re-encrypted: %v", err)
	}
	if decrypted2 != plaintext {
		t.Errorf("got %q, want %q", decrypted2, plaintext)
	}
}

func TestInvalidKeyLength(t *testing.T) {
	keys := map[string]string{
		"v1": "tooshort",
	}
	_, err := NewEncryptor(keys, "v1")
	if err == nil {
		t.Error("expected error for short key")
	}
}

func TestDecryptInvalidFormat(t *testing.T) {
	keys := map[string]string{
		"v1": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	enc, _ := NewEncryptor(keys, "v1")

	_, err := enc.Decrypt("no-version-prefix")
	if err == nil {
		t.Error("expected error for missing version prefix")
	}

	_, err = enc.Decrypt("v99:somebase64data")
	if err == nil {
		t.Error("expected error for unknown version")
	}
}
