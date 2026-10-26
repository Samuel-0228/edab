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

func TestCredentialsStoreUpdateAndErrors(t *testing.T) {
	ctx := context.Background()
	key := DeriveKey("update-test-key")
	store := NewMemoryStore(key)

	// Create
	c, err := store.Create(ctx, &Credential{
		Name:        "Initial Name",
		Integration: "github",
	}, map[string]string{
		"token": "ghp_111222333444555",
	})
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	// Update name and token
	c.Name = "Updated GitHub"
	updated, err := store.Update(ctx, c, map[string]string{
		"token": "ghp_999888777666555",
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "Updated GitHub" {
		t.Errorf("expected updated name, got %s", updated.Name)
	}

	// Verify updated secret is decrypted correctly
	decrypted, err := store.GetDecrypted(ctx, c.ID)
	if err != nil || decrypted["token"] != "ghp_999888777666555" {
		t.Fatalf("expected updated decrypted token, got: %v", decrypted)
	}

	// Update nonexistent
	_, err = store.Update(ctx, &Credential{ID: "nonexistent"}, map[string]string{"foo": "bar"})
	if err != ErrCredentialNotFound {
		t.Fatalf("expected ErrCredentialNotFound on update nonexistent, got %v", err)
	}

	// GetDecrypted nonexistent
	_, err = store.GetDecrypted(ctx, "nonexistent")
	if err != ErrCredentialNotFound {
		t.Fatalf("expected ErrCredentialNotFound on GetDecrypted nonexistent, got %v", err)
	}

	// Delete nonexistent
	err = store.Delete(ctx, "nonexistent")
	if err != ErrCredentialNotFound {
		t.Fatalf("expected ErrCredentialNotFound on Delete nonexistent, got %v", err)
	}
}

func TestCryptoErrorCases(t *testing.T) {
	// 1. Invalid key length for Encrypt
	_, _, err := Encrypt([]byte("too-short"), []byte("data"))
	if err == nil {
		t.Fatal("expected error on invalid key length for Encrypt")
	}

	// 2. Invalid key length for Decrypt
	_, err = Decrypt([]byte("too-short"), []byte("cipher"), []byte("nonce"))
	if err == nil {
		t.Fatal("expected error on invalid key length for Decrypt")
	}

	// 3. Invalid nonce size for Decrypt
	key := DeriveKey("valid-32-byte-key-master-test-key")
	_, err = Decrypt(key, []byte("cipher"), []byte("short-nonce"))
	if err == nil {
		t.Fatal("expected error on invalid nonce size for Decrypt")
	}

	// 4. DeriveKey deterministic and 32 bytes
	k1 := DeriveKey("same-secret")
	k2 := DeriveKey("same-secret")
	if len(k1) != 32 || len(k2) != 32 || string(k1) != string(k2) {
		t.Fatalf("DeriveKey must produce deterministic 32-byte keys")
	}

	// 5. DeriveKey with empty string uses default fallback
	kDefault := DeriveKey("")
	if len(kDefault) != 32 {
		t.Fatalf("DeriveKey('') must produce 32-byte key, got %d", len(kDefault))
	}
}

func TestMemoryStoreValidationErrors(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(DeriveKey("key"))

	// Empty name
	_, err := store.Create(ctx, &Credential{Name: "", Integration: "telegram"}, nil)
	if err == nil {
		t.Fatal("expected error on empty credential name")
	}

	// Empty integration
	_, err = store.Create(ctx, &Credential{Name: "cred", Integration: ""}, nil)
	if err == nil {
		t.Fatal("expected error on empty credential integration")
	}

	// GetByID nonexistent
	_, err = store.GetByID(ctx, "nonexistent")
	if err != ErrCredentialNotFound {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}
