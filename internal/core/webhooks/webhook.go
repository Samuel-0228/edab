package webhooks

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"kerob/internal/core/event"
)

var (
	ErrWebhookNotFound    = errors.New("webhook not found")
	ErrWebhookDisabled    = errors.New("webhook is disabled")
	ErrInvalidSecretToken = errors.New("invalid or missing webhook secret token")
	ErrPayloadTooLarge    = errors.New("payload exceeds maximum allowed size")
	ErrRateLimitExceeded  = errors.New("webhook rate limit exceeded")
)

const MaxWebhookPayloadBytes = 1024 * 1024 // 1 MB

// Webhook defines an inbound generic webhook endpoint configuration.
type Webhook struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	SecretToken        string    `json:"secret_token,omitempty"`
	RateLimitPerMinute int       `json:"rate_limit_per_minute"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Store defines persistence operations for Webhooks.
type Store interface {
	Create(ctx context.Context, wh *Webhook) (*Webhook, error)
	GetByID(ctx context.Context, id string) (*Webhook, error)
	List(ctx context.Context) ([]*Webhook, error)
	Update(ctx context.Context, wh *Webhook) (*Webhook, error)
	Delete(ctx context.Context, id string) error
}

// MemoryStore provides in-memory storage for webhooks.
type MemoryStore struct {
	mu       sync.RWMutex
	webhooks map[string]*Webhook
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		webhooks: make(map[string]*Webhook),
	}
}

func (s *MemoryStore) Create(ctx context.Context, wh *Webhook) (*Webhook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wh.Name == "" {
		return nil, fmt.Errorf("webhook name is required")
	}
	if wh.ID == "" {
		wh.ID = uuid.NewString()
	}
	if wh.RateLimitPerMinute <= 0 {
		wh.RateLimitPerMinute = 60
	}
	now := time.Now().UTC()
	wh.CreatedAt = now
	wh.UpdatedAt = now

	s.webhooks[wh.ID] = wh
	return wh, nil
}

func (s *MemoryStore) GetByID(ctx context.Context, id string) (*Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wh, ok := s.webhooks[id]
	if !ok {
		return nil, ErrWebhookNotFound
	}
	return wh, nil
}

func (s *MemoryStore) List(ctx context.Context) ([]*Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Webhook, 0, len(s.webhooks))
	for _, wh := range s.webhooks {
		list = append(list, wh)
	}
	return list, nil
}

func (s *MemoryStore) Update(ctx context.Context, wh *Webhook) (*Webhook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.webhooks[wh.ID]
	if !ok {
		return nil, ErrWebhookNotFound
	}
	existing.Name = wh.Name
	existing.SecretToken = wh.SecretToken
	existing.RateLimitPerMinute = wh.RateLimitPerMinute
	existing.Enabled = wh.Enabled
	existing.UpdatedAt = time.Now().UTC()

	return existing, nil
}

func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.webhooks[id]; !ok {
		return ErrWebhookNotFound
	}
	delete(s.webhooks, id)
	return nil
}

// Authenticate verifies the incoming request against the webhook's configured secret token.
func Authenticate(wh *Webhook, r *http.Request) error {
	if wh.SecretToken == "" {
		// No secret configured, unrestricted for this specific webhook
		return nil
	}

	// 1. Check header X-Webhook-Token
	if token := r.Header.Get("X-Webhook-Token"); token != "" {
		if subtle.ConstantTimeCompare([]byte(token), []byte(wh.SecretToken)) == 1 {
			return nil
		}
	}

	// 2. Check Authorization header: Bearer <token>
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(wh.SecretToken)) == 1 {
			return nil
		}
	}

	// 3. Check query param: ?token=<token> or ?secret=<token>
	queryToken := r.URL.Query().Get("token")
	if queryToken == "" {
		queryToken = r.URL.Query().Get("secret")
	}
	if queryToken != "" {
		if subtle.ConstantTimeCompare([]byte(queryToken), []byte(wh.SecretToken)) == 1 {
			return nil
		}
	}

	return ErrInvalidSecretToken
}

// NormalizeWebhookEvent converts incoming webhook body and headers into a standardized Event.
func NormalizeWebhookEvent(wh *Webhook, rawBody []byte, headers http.Header) (*event.Event, error) {
	metadata := map[string]any{
		"webhook_id":   wh.ID,
		"webhook_name": wh.Name,
		"user_agent":   headers.Get("User-Agent"),
		"content_type": headers.Get("Content-Type"),
	}

	externalID := fmt.Sprintf("wh:%s:%d", wh.ID, time.Now().UnixNano())
	eventType := "webhook.received"

	// Check if custom event type header is provided
	if customType := headers.Get("X-Event-Type"); customType != "" {
		eventType = customType
	}

	return event.New(wh.ID, externalID, eventType, rawBody, metadata)
}
