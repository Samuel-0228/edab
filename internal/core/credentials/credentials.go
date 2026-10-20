package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCredentialNotFound = errors.New("credential not found")
)

// Credential represents securely stored credentials for an integration.
// Encrypted at rest, plaintext is never serialized to JSON.
type Credential struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Integration  string            `json:"integration"` // e.g. "telegram", "discord", "github", "http"
	KeyID        string            `json:"key_id"`
	MaskedFields map[string]string `json:"masked_fields"` // e.g. {"token": "••••••••1234"}
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`

	// Unexported encrypted storage fields
	EncryptedData []byte `json:"-"`
	Nonce         []byte `json:"-"`
}

// Store defines persistent storage operations for credentials.
type Store interface {
	Create(ctx context.Context, cred *Credential, rawValues map[string]string) (*Credential, error)
	GetByID(ctx context.Context, id string) (*Credential, error)
	GetDecrypted(ctx context.Context, id string) (map[string]string, error)
	List(ctx context.Context) ([]*Credential, error)
	Update(ctx context.Context, cred *Credential, rawValues map[string]string) (*Credential, error)
	Delete(ctx context.Context, id string) error
}

// MemoryStore implements an in-memory Store for testing and decoupled operation.
type MemoryStore struct {
	mu     sync.RWMutex
	key    []byte
	creds  map[string]*Credential
	values map[string]map[string]string
}

func NewMemoryStore(encryptionKey []byte) *MemoryStore {
	return &MemoryStore{
		key:    encryptionKey,
		creds:  make(map[string]*Credential),
		values: make(map[string]map[string]string),
	}
}

func (s *MemoryStore) Create(ctx context.Context, cred *Credential, rawValues map[string]string) (*Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cred.Name == "" {
		return nil, fmt.Errorf("credential name is required")
	}
	if cred.Integration == "" {
		return nil, fmt.Errorf("credential integration is required")
	}

	if cred.ID == "" {
		cred.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	cred.CreatedAt = now
	cred.UpdatedAt = now
	cred.KeyID = "default"

	// Mask values
	cred.MaskedFields = make(map[string]string)
	for k, v := range rawValues {
		cred.MaskedFields[k] = MaskSecret(v)
	}

	// Encrypt payload
	rawBytes, err := json.Marshal(rawValues)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credential data: %w", err)
	}
	ciphertext, nonce, err := Encrypt(s.key, rawBytes)
	if err != nil {
		return nil, fmt.Errorf("encryption error: %w", err)
	}
	cred.EncryptedData = ciphertext
	cred.Nonce = nonce

	s.creds[cred.ID] = cred
	// Save copy of raw values
	copied := make(map[string]string)
	for k, v := range rawValues {
		copied[k] = v
	}
	s.values[cred.ID] = copied

	return cred, nil
}

func (s *MemoryStore) GetByID(ctx context.Context, id string) (*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.creds[id]
	if !ok {
		return nil, ErrCredentialNotFound
	}
	return c, nil
}

func (s *MemoryStore) GetDecrypted(ctx context.Context, id string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.creds[id]
	if !ok {
		return nil, ErrCredentialNotFound
	}

	plaintext, err := Decrypt(s.key, c.EncryptedData, c.Nonce)
	if err != nil {
		return nil, err
	}

	var res map[string]string
	if err := json.Unmarshal(plaintext, &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted credentials: %w", err)
	}
	return res, nil
}

func (s *MemoryStore) List(ctx context.Context) ([]*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Credential, 0, len(s.creds))
	for _, c := range s.creds {
		list = append(list, c)
	}
	return list, nil
}

func (s *MemoryStore) Update(ctx context.Context, cred *Credential, rawValues map[string]string) (*Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.creds[cred.ID]
	if !ok {
		return nil, ErrCredentialNotFound
	}

	existing.Name = cred.Name
	existing.Integration = cred.Integration
	existing.UpdatedAt = time.Now().UTC()

	if len(rawValues) > 0 {
		existing.MaskedFields = make(map[string]string)
		for k, v := range rawValues {
			existing.MaskedFields[k] = MaskSecret(v)
		}
		rawBytes, err := json.Marshal(rawValues)
		if err != nil {
			return nil, err
		}
		ciphertext, nonce, err := Encrypt(s.key, rawBytes)
		if err != nil {
			return nil, err
		}
		existing.EncryptedData = ciphertext
		existing.Nonce = nonce
		s.values[cred.ID] = rawValues
	}

	return existing, nil
}

func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.creds[id]; !ok {
		return ErrCredentialNotFound
	}
	delete(s.creds, id)
	delete(s.values, id)
	return nil
}
