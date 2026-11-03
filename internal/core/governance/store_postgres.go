package governance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PostgresStore provides a PostgreSQL-backed implementation of Store.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore initializes a PostgreSQL governance policy store.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// GetPolicy retrieves the policy for a tenant from PostgreSQL, falling back to defaults if not found.
func (s *PostgresStore) GetPolicy(ctx context.Context, tenantID string) (*Policy, error) {
	query := `
		SELECT tenant_id, allowed_connectors, allowed_outbound_domains, max_workflow_retries,
		       max_payload_size_bytes, data_retention_days, require_approval_irreversible,
		       mask_pii, mask_secrets, blocked_keywords, allowed_environments,
		       enforcement_mode, created_at, updated_at
		FROM governance_policies
		WHERE tenant_id = $1;
	`
	row := s.db.QueryRowContext(ctx, query, tenantID)

	var p Policy
	var rawConnectors, rawDomains, rawKeywords, rawEnvs []byte

	err := row.Scan(
		&p.TenantID, &rawConnectors, &rawDomains, &p.MaxWorkflowRetries,
		&p.MaxPayloadSizeBytes, &p.DataRetentionDays, &p.RequireApprovalForIrreversibleActions,
		&p.MaskPII, &p.MaskSecrets, &rawKeywords, &rawEnvs,
		&p.EnforcementMode, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPolicy(tenantID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get governance policy: %w", err)
	}

	_ = json.Unmarshal(rawConnectors, &p.AllowedConnectors)
	_ = json.Unmarshal(rawDomains, &p.AllowedOutboundDomains)
	_ = json.Unmarshal(rawKeywords, &p.BlockedKeywords)
	_ = json.Unmarshal(rawEnvs, &p.AllowedEnvironments)

	return &p, nil
}

// UpsertPolicy inserts or updates the tenant's governance policy.
func (s *PostgresStore) UpsertPolicy(ctx context.Context, policy *Policy) (*Policy, error) {
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

	rawConnectors, _ := json.Marshal(cloned.AllowedConnectors)
	rawDomains, _ := json.Marshal(cloned.AllowedOutboundDomains)
	rawKeywords, _ := json.Marshal(cloned.BlockedKeywords)
	rawEnvs, _ := json.Marshal(cloned.AllowedEnvironments)

	query := `
		INSERT INTO governance_policies (
			tenant_id, allowed_connectors, allowed_outbound_domains, max_workflow_retries,
			max_payload_size_bytes, data_retention_days, require_approval_irreversible,
			mask_pii, mask_secrets, blocked_keywords, allowed_environments, enforcement_mode,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (tenant_id) DO UPDATE SET
			allowed_connectors = EXCLUDED.allowed_connectors,
			allowed_outbound_domains = EXCLUDED.allowed_outbound_domains,
			max_workflow_retries = EXCLUDED.max_workflow_retries,
			max_payload_size_bytes = EXCLUDED.max_payload_size_bytes,
			data_retention_days = EXCLUDED.data_retention_days,
			require_approval_irreversible = EXCLUDED.require_approval_irreversible,
			mask_pii = EXCLUDED.mask_pii,
			mask_secrets = EXCLUDED.mask_secrets,
			blocked_keywords = EXCLUDED.blocked_keywords,
			allowed_environments = EXCLUDED.allowed_environments,
			enforcement_mode = EXCLUDED.enforcement_mode,
			updated_at = EXCLUDED.updated_at
		RETURNING created_at, updated_at;
	`

	err := s.db.QueryRowContext(ctx, query,
		cloned.TenantID, rawConnectors, rawDomains, cloned.MaxWorkflowRetries,
		cloned.MaxPayloadSizeBytes, cloned.DataRetentionDays, cloned.RequireApprovalForIrreversibleActions,
		cloned.MaskPII, cloned.MaskSecrets, rawKeywords, rawEnvs, cloned.EnforcementMode,
		cloned.CreatedAt, cloned.UpdatedAt,
	).Scan(&cloned.CreatedAt, &cloned.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to upsert governance policy: %w", err)
	}

	return cloned, nil
}

// DeletePolicy resets a tenant policy back to defaults by removing custom record.
func (s *PostgresStore) DeletePolicy(ctx context.Context, tenantID string) error {
	query := `DELETE FROM governance_policies WHERE tenant_id = $1;`
	_, err := s.db.ExecContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete governance policy: %w", err)
	}
	return nil
}
