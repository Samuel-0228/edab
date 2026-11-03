package governance

import (
	"context"
	"sync"
	"time"
)

// MemoryStore provides a thread-safe, in-memory implementation of Store.
type MemoryStore struct {
	mu       sync.RWMutex
	policies map[string]*Policy
}

// NewMemoryStore initializes an in-memory governance policy store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		policies: make(map[string]*Policy),
	}
}

// GetPolicy retrieves the policy for a tenant, falling back to safe defaults if none has been explicitly configured.
func (s *MemoryStore) GetPolicy(_ context.Context, tenantID string) (*Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if p, ok := s.policies[tenantID]; ok {
		return clonePolicy(p), nil
	}
	return DefaultPolicy(tenantID), nil
}

// UpsertPolicy creates or updates the governance policy for a tenant.
func (s *MemoryStore) UpsertPolicy(_ context.Context, policy *Policy) (*Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cloned := clonePolicy(policy)
	if cloned.CreatedAt.IsZero() {
		cloned.CreatedAt = now
	}
	cloned.UpdatedAt = now

	if cloned.EnforcementMode != ModeAudit && cloned.EnforcementMode != ModeEnforce {
		cloned.EnforcementMode = ModeEnforce
	}
	if cloned.MaxPayloadSizeBytes <= 0 {
		cloned.MaxPayloadSizeBytes = 1048576
	}
	if cloned.DataRetentionDays <= 0 {
		cloned.DataRetentionDays = 90
	}
	if cloned.MaxWorkflowRetries < 0 {
		cloned.MaxWorkflowRetries = 0
	}

	s.policies[cloned.TenantID] = cloned
	return clonePolicy(cloned), nil
}

// DeletePolicy deletes any custom governance policy for a tenant, resetting to defaults.
func (s *MemoryStore) DeletePolicy(_ context.Context, tenantID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.policies, tenantID)
	return nil
}

func clonePolicy(p *Policy) *Policy {
	if p == nil {
		return nil
	}
	cp := *p
	cp.AllowedConnectors = append([]string(nil), p.AllowedConnectors...)
	cp.AllowedOutboundDomains = append([]string(nil), p.AllowedOutboundDomains...)
	cp.BlockedKeywords = append([]string(nil), p.BlockedKeywords...)
	cp.AllowedEnvironments = append([]string(nil), p.AllowedEnvironments...)
	return &cp
}
