package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrDecryptionFailed = errors.New("failed to decrypt credentials: authentication or key mismatch")
	ErrInvalidKey       = errors.New("encryption key must be non-empty")
)

// DeriveKey generates a 32-byte AES-256 key from any input string.
func DeriveKey(secret string) []byte {
	if secret == "" {
		secret = "kerob-default-local-encryption-key-32bytes!!"
	}
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// Encrypt encrypts plaintext using AES-256-GCM with a newly generated random nonce.
func Encrypt(key []byte, plaintext []byte) ([]byte, []byte, error) {
	if len(key) != 32 {
		return nil, nil, fmt.Errorf("invalid key length: expected 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts AES-256-GCM ciphertext using the given key and nonce.
func Decrypt(key []byte, ciphertext []byte, nonce []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid key length: expected 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: expected %d, got %d", gcm.NonceSize(), len(nonce))
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// MaskSecret returns a redacted string showing at most the last 4 characters.
// Sensitive secrets are never exposed in plaintext in logs, APIs, or UI.
func MaskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if len(secret) == 0 {
		return ""
	}
	if len(secret) <= 4 {
		return "••••••••"
	}
	visibleLen := 4
	if len(secret) > 20 {
		visibleLen = 6
	}
	return "••••••••" + secret[len(secret)-visibleLen:]
}
