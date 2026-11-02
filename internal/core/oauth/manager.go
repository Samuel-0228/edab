package oauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"kerob/internal/core/credentials"
	"kerob/pkg/connector"
)

var (
	ErrConnectorNotFound  = errors.New("connector not found")
	ErrNotOAuthProvider   = errors.New("connector does not support OAuth2")
	ErrCredentialNotFound = errors.New("credential not found")
	ErrNoRefreshToken     = errors.New("credential does not have a refresh token")
	ErrStateMismatch      = errors.New("oauth callback state connector or tenant mismatch")
)

// Manager coordinates OAuth authorization flows, token exchanges, encrypted storage,
// refresh-token rotation, and token revocations across registered connectors.
type Manager struct {
	registry     *connector.Registry
	credStore    credentials.Store
	stateManager *StateManager
}

// NewManager creates a new OAuth Manager.
func NewManager(registry *connector.Registry, credStore credentials.Store, stateManager *StateManager) *Manager {
	if registry == nil {
		registry = connector.Default()
	}
	if stateManager == nil {
		stateManager = NewStateManager(10 * time.Minute)
	}
	return &Manager{
		registry:     registry,
		credStore:    credStore,
		stateManager: stateManager,
	}
}

// StateManager returns the underlying state manager for inspection and testing.
func (m *Manager) StateManager() *StateManager {
	return m.stateManager
}

// GetAuthorizeURL generates a cryptographically secure CSRF state and constructs
// the upstream authorization URL for the specified connector.
func (m *Manager) GetAuthorizeURL(ctx context.Context, connectorID, tenantID, userID, redirectURI string, extraParams map[string]string) (string, string, error) {
	conn, ok := m.registry.Get(connectorID)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrConnectorNotFound, connectorID)
	}

	provider, ok := conn.(connector.OAuth2Provider)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrNotOAuthProvider, connectorID)
	}

	state, err := m.stateManager.Generate(connectorID, tenantID, userID, redirectURI)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate oauth state: %w", err)
	}

	authURL := provider.GetAuthorizationURL(state.State, redirectURI, extraParams)
	return authURL, state.State, nil
}

// HandleCallback processes the authorization code returned by upstream OAuth provider,
// verifies the CSRF state token, exchanges code for tokens, and securely encrypts the tokens.
func (m *Manager) HandleCallback(ctx context.Context, connectorID, stateToken, code, redirectURI string) (*credentials.Credential, error) {
	if code == "" {
		return nil, errors.New("authorization code is required")
	}

	// 1. Consume and validate CSRF state token
	state, err := m.stateManager.ValidateAndConsume(stateToken)
	if err != nil {
		return nil, fmt.Errorf("csrf state verification failed: %w", err)
	}

	// 2. Validate that state was issued for this connector
	if state.ConnectorID != connectorID {
		return nil, ErrStateMismatch
	}

	// 3. Resolve connector
	conn, ok := m.registry.Get(connectorID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrConnectorNotFound, connectorID)
	}

	provider, ok := conn.(connector.OAuth2Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotOAuthProvider, connectorID)
	}

	// Use saved redirect URI if not provided
	if redirectURI == "" {
		redirectURI = state.RedirectURI
	}

	// 4. Exchange authorization code for token
	token, err := provider.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	if token.AccessToken == "" {
		return nil, errors.New("upstream provider returned empty access token")
	}

	// 5. Encrypt and store credential securely
	credName := fmt.Sprintf("%s OAuth (%s)", conn.Manifest().Name, time.Now().UTC().Format("2006-01-02 15:04"))
	cred := &credentials.Credential{
		TenantID:    state.TenantID,
		Name:        credName,
		Integration: connectorID,
	}

	// Raw values are encrypted using AES-256-GCM by credStore.Create
	// Raw tokens are NEVER exposed or retained in the returned Credential struct
	stored, err := m.credStore.Create(ctx, cred, token.ToCredentialMap())
	if err != nil {
		return nil, fmt.Errorf("failed to store encrypted oauth credential: %w", err)
	}

	return stored, nil
}

// RefreshToken performs refresh-token rotation for an existing OAuth credential.
func (m *Manager) RefreshToken(ctx context.Context, connectorID, credentialID string) (*credentials.Credential, error) {
	// 1. Retrieve current decrypted tokens from vault
	rawValues, err := m.credStore.GetDecrypted(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credential: %w", err)
	}

	currentToken := connector.FromCredentialMap(rawValues)
	if currentToken.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}

	// 2. Resolve connector
	conn, ok := m.registry.Get(connectorID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrConnectorNotFound, connectorID)
	}

	provider, ok := conn.(connector.OAuth2Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotOAuthProvider, connectorID)
	}

	// 3. Exchange refresh token with upstream provider
	newToken, err := provider.RefreshToken(ctx, currentToken.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed at upstream provider: %w", err)
	}

	// If provider did not return a new refresh token (non-rotating provider), keep existing
	if newToken.RefreshToken == "" {
		newToken.RefreshToken = currentToken.RefreshToken
	}

	// 4. Update encrypted credential in store
	cred, err := m.credStore.GetByID(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("failed to load credential metadata: %w", err)
	}

	updated, err := m.credStore.Update(ctx, cred, newToken.ToCredentialMap())
	if err != nil {
		return nil, fmt.Errorf("failed to save rotated credential: %w", err)
	}

	return updated, nil
}

// RevokeToken revokes the token at the upstream OAuth server and deletes the stored credential.
func (m *Manager) RevokeToken(ctx context.Context, connectorID, credentialID string) error {
	rawValues, err := m.credStore.GetDecrypted(ctx, credentialID)
	if err != nil {
		return fmt.Errorf("failed to load credential for revocation: %w", err)
	}

	token := connector.FromCredentialMap(rawValues)

	// Call provider revocation if implemented
	if conn, ok := m.registry.Get(connectorID); ok {
		if provider, ok := conn.(connector.OAuth2Provider); ok {
			revokeTarget := token.AccessToken
			if revokeTarget == "" {
				revokeTarget = token.RefreshToken
			}
			if revokeTarget != "" {
				_ = provider.RevokeToken(ctx, revokeTarget)
			}
		}
	}

	// Delete from credential vault
	if err := m.credStore.Delete(ctx, credentialID); err != nil {
		return fmt.Errorf("failed to delete credential from store: %w", err)
	}

	return nil
}
