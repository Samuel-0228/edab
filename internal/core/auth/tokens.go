package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// GenerateAPIToken generates a new secure API token string, its human-readable prefix, and its SHA-256 hash.
// Token format: krb_<prefix>_<32-byte-hex> (e.g., krb_live_a1b2c3d4e5f6...)
func GenerateAPIToken(prefix string) (rawToken string, tokenPrefix string, tokenHash string, err error) {
	if prefix == "" {
		prefix = "live"
	}
	prefix = strings.TrimSpace(prefix)

	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("failed to generate random token bytes: %w", err)
	}

	secretPart := hex.EncodeToString(b)
	rawToken = fmt.Sprintf("krb_%s_%s", prefix, secretPart)
	tokenPrefix = fmt.Sprintf("krb_%s_%s...", prefix, secretPart[:6])
	tokenHash = HashToken(rawToken)

	return rawToken, tokenPrefix, tokenHash, nil
}

// HashToken computes a deterministic SHA-256 hex digest of the raw token.
func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawToken)))
	return hex.EncodeToString(sum[:])
}

// VerifyAPIToken verifies a raw bearer token against its expected SHA-256 hash in constant time.
func VerifyAPIToken(rawToken, expectedHash string) bool {
	actualHash := HashToken(rawToken)
	return subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) == 1
}
