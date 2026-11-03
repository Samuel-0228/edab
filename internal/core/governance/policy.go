package governance

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	ModeEnforce = "enforce"
	ModeAudit   = "audit"
)

// Policy defines the enterprise governance and security controls for a tenant.
type Policy struct {
	TenantID                              string    `json:"tenant_id"`
	AllowedConnectors                     []string  `json:"allowed_connectors"`
	AllowedOutboundDomains                []string  `json:"allowed_outbound_domains"`
	MaxWorkflowRetries                    int       `json:"max_workflow_retries"`
	MaxPayloadSizeBytes                   int64     `json:"max_payload_size_bytes"`
	DataRetentionDays                     int       `json:"data_retention_days"`
	RequireApprovalForIrreversibleActions bool      `json:"require_approval_irreversible"`
	MaskPII                               bool      `json:"mask_pii"`
	MaskSecrets                           bool      `json:"mask_secrets"`
	BlockedKeywords                       []string  `json:"blocked_keywords"`
	AllowedEnvironments                   []string  `json:"allowed_environments"`
	EnforcementMode                       string    `json:"enforcement_mode"` // "enforce" or "audit"
	CreatedAt                             time.Time `json:"created_at"`
	UpdatedAt                             time.Time `json:"updated_at"`
}

// DefaultPolicy returns safe, enterprise-ready default policies for a tenant.
func DefaultPolicy(tenantID string) *Policy {
	now := time.Now().UTC()
	return &Policy{
		TenantID:                              tenantID,
		AllowedConnectors:                     []string{},
		AllowedOutboundDomains:                []string{},
		MaxWorkflowRetries:                    5,
		MaxPayloadSizeBytes:                   1048576, // 1MB default
		DataRetentionDays:                     90,
		RequireApprovalForIrreversibleActions: false,
		MaskPII:                               true,
		MaskSecrets:                           true,
		BlockedKeywords:                       []string{},
		AllowedEnvironments:                   []string{},
		EnforcementMode:                       ModeEnforce,
		CreatedAt:                             now,
		UpdatedAt:                             now,
	}
}

// PolicyViolation represents an individual rule violation detected by the policy engine.
type PolicyViolation struct {
	Rule    string         `json:"rule"`
	Field   string         `json:"field"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// PolicyViolationError represents an aggregated set of policy violations.
type PolicyViolationError struct {
	TenantID   string            `json:"tenant_id"`
	Violations []PolicyViolation `json:"violations"`
}

func (e *PolicyViolationError) Error() string {
	if len(e.Violations) == 0 {
		return "policy violation detected"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("policy violations for tenant %s: ", e.TenantID))
	for i, v := range e.Violations {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s", v.Rule, v.Field, v.Message))
	}
	return sb.String()
}

// Store defines persistence operations for tenant governance policies.
type Store interface {
	GetPolicy(ctx context.Context, tenantID string) (*Policy, error)
	UpsertPolicy(ctx context.Context, policy *Policy) (*Policy, error)
	DeletePolicy(ctx context.Context, tenantID string) error
}
