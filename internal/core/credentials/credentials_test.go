package credentials

import (
	"context"
	"testing"
)

func TestCryptoEncryptionDecryption(t *testing.T) {
	key := DeriveKey("super-secret-test-master-key")
	plaintext := []byte("secret_api_token_123456789")

	ciphertext, nonce, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("ciphertext is empty")
	}

	decrypted, err := Decrypt(key, ciphertext, nonce)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("expected decrypted %s, got %s", string(plaintext), string(decrypted))
	}

	// Test wrong key
	wrongKey := DeriveKey("wrong-key")
	_, err = Decrypt(wrongKey, ciphertext, nonce)
	if err == nil {
		t.Fatal("expected decryption failure with wrong key, got nil")
	}

	// Test tampered ciphertext
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[0] ^= 0xFF
	_, err = Decrypt(key, tampered, nonce)
	if err == nil {
		t.Fatal("expected decryption failure with tampered ciphertext, got nil")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"abcd", "••••••••"},
		{"bot123456:ABC-DEF1234ghIkl-zyx57W2v1u", "••••••••7W2v1u"},
		{"ghp_1234567890abcdef", "••••••••cdef"},
	}

	for _, tt := range tests {
		actual := MaskSecret(tt.input)
		if actual != tt.expected {
			t.Errorf("MaskSecret(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestMemoryStoreCRUD(t *testing.T) {
	ctx := context.Background()
	key := DeriveKey("store-test-key")
	store := NewMemoryStore(key)

	cred, err := store.Create(ctx, &Credential{
		Name:        "Production Discord Bot",
		Integration: "discord",
	}, map[string]string{
		"bot_token":   "discord_secret_bot_token_987654321",
		"webhook_url": "https://discord.com/api/webhooks/123/xyz",
	})
	if err != nil {
		t.Fatalf("failed to create credential: %v", err)
	}

	if cred.ID == "" {
		t.Fatal("expected generated credential ID")
	}
	if cred.MaskedFields["bot_token"] == "discord_secret_bot_token_987654321" {
		t.Fatal("masked field must not contain raw secret")
	}

	// Verify GetByID returns masked fields only
	fetched, err := store.GetByID(ctx, cred.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.Name != "Production Discord Bot" {
		t.Errorf("expected name 'Production Discord Bot', got %s", fetched.Name)
	}

	// Verify GetDecrypted returns plaintext
	decrypted, err := store.GetDecrypted(ctx, cred.ID)
	if err != nil {
		t.Fatalf("GetDecrypted failed: %v", err)
	}
	if decrypted["bot_token"] != "discord_secret_bot_token_987654321" {
		t.Fatalf("expected decrypted bot_token, got %s", decrypted["bot_token"])
	}

	// Verify List
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(list))
	}

	// Delete
	if err := store.Delete(ctx, cred.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = store.GetByID(ctx, cred.ID)
	if err != ErrCredentialNotFound {
		t.Fatalf("expected ErrCredentialNotFound after delete, got %v", err)
	}
}
