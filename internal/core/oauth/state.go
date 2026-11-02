package oauth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrInvalidState = errors.New("invalid or unissued oauth state token")
	ErrStateExpired = errors.New("oauth state token has expired (ttl exceeded)")
)

// OAuthState holds the security context of an in-flight OAuth authorization request.
type OAuthState struct {
	State       string    `json:"state"`
	ConnectorID string    `json:"connector_id"`
	TenantID    string    `json:"tenant_id"`
	UserID      string    `json:"user_id"`
	RedirectURI string    `json:"redirect_uri"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// StateManager generates, validates, and consumes high-entropy CSRF protection state tokens.
type StateManager struct {
	mu     sync.Mutex
	states map[string]*OAuthState
	ttl    time.Duration
}

// NewStateManager creates a StateManager with the specified TTL (default 10 minutes).
func NewStateManager(ttl time.Duration) *StateManager {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &StateManager{
		states: make(map[string]*OAuthState),
		ttl:    ttl,
	}
}

// Generate creates a cryptographically secure, non-guessable random state token.
func (sm *StateManager) Generate(connectorID, tenantID, userID, redirectURI string) (*OAuthState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Periodic purge of expired tokens
	sm.cleanupExpiredLocked()

	// 32 random bytes -> 64 character hex string
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure random state: %w", err)
	}
	stateToken := hex.EncodeToString(bytes)

	now := time.Now().UTC()
	st := &OAuthState{
		State:       stateToken,
		ConnectorID: connectorID,
		TenantID:    tenantID,
		UserID:      userID,
		RedirectURI: redirectURI,
		CreatedAt:   now,
		ExpiresAt:   now.Add(sm.ttl),
	}

	sm.states[stateToken] = st
	return st, nil
}

// ValidateAndConsume checks validity and single-use consumes the state token.
// Defends against replay attacks and cross-site request forgery.
func (sm *StateManager) ValidateAndConsume(stateToken string) (*OAuthState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if stateToken == "" {
		return nil, ErrInvalidState
	}

	st, ok := sm.states[stateToken]
	if !ok {
		return nil, ErrInvalidState
	}

	// Always delete token upon lookup (single-use guarantee)
	delete(sm.states, stateToken)

	if time.Now().UTC().After(st.ExpiresAt) {
		return nil, ErrStateExpired
	}

	return st, nil
}

func (sm *StateManager) cleanupExpiredLocked() {
	now := time.Now().UTC()
	for k, v := range sm.states {
		if now.After(v.ExpiresAt) {
			delete(sm.states, k)
		}
	}
}
