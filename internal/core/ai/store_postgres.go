package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PostgresStore provides a PostgreSQL-backed implementation of Store.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore initializes a PostgreSQL proposal store.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateProposal(ctx context.Context, p *Proposal) (*Proposal, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.Status == "" {
		p.Status = StatusPendingApproval
	}

	contentBytes := p.Content
	if len(contentBytes) == 0 {
		contentBytes = []byte("{}")
	}

	valBytes, err := json.Marshal(p.Validation)
	if err != nil {
		valBytes = []byte("{}")
	}

	query := `
		INSERT INTO ai_proposals (
			id, tenant_id, task, status, prompt, explanation,
			content, validation, model_used, tokens_used, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at;
	`

	err = s.db.QueryRowContext(ctx, query,
		p.ID, p.TenantID, string(p.Task), string(p.Status), p.Prompt, p.Explanation,
		contentBytes, valBytes, p.ModelUsed, p.TokensUsed, p.CreatedAt,
	).Scan(&p.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create ai proposal: %w", err)
	}

	return cloneProposal(p), nil
}

func (s *PostgresStore) GetProposal(ctx context.Context, tenantID, id string) (*Proposal, error) {
	query := `
		SELECT id, tenant_id, task, status, prompt, explanation,
		       content, validation, model_used, tokens_used, created_at,
		       decided_at, decided_by
		FROM ai_proposals
		WHERE tenant_id = $1 AND id = $2;
	`
	row := s.db.QueryRowContext(ctx, query, tenantID, id)

	var p Proposal
	var taskStr, statusStr string
	var rawContent, rawValidation []byte
	var decidedAt sql.NullTime
	var decidedBy sql.NullString

	err := row.Scan(
		&p.ID, &p.TenantID, &taskStr, &statusStr, &p.Prompt, &p.Explanation,
		&rawContent, &rawValidation, &p.ModelUsed, &p.TokensUsed, &p.CreatedAt,
		&decidedAt, &decidedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProposalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan ai proposal: %w", err)
	}

	p.Task = TaskType(taskStr)
	p.Status = ProposalStatus(statusStr)
	p.Content = rawContent
	_ = json.Unmarshal(rawValidation, &p.Validation)

	if decidedAt.Valid {
		p.DecidedAt = &decidedAt.Time
	}
	if decidedBy.Valid {
		p.DecidedBy = &decidedBy.String
	}

	return &p, nil
}

func (s *PostgresStore) ListProposals(ctx context.Context, tenantID string, limit, offset int) ([]*Proposal, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT id, tenant_id, task, status, prompt, explanation,
		       content, validation, model_used, tokens_used, created_at,
		       decided_at, decided_by
		FROM ai_proposals
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3;
	`
	rows, err := s.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list ai proposals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Proposal
	for rows.Next() {
		var p Proposal
		var taskStr, statusStr string
		var rawContent, rawValidation []byte
		var decidedAt sql.NullTime
		var decidedBy sql.NullString

		if err := rows.Scan(
			&p.ID, &p.TenantID, &taskStr, &statusStr, &p.Prompt, &p.Explanation,
			&rawContent, &rawValidation, &p.ModelUsed, &p.TokensUsed, &p.CreatedAt,
			&decidedAt, &decidedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan proposal: %w", err)
		}
		p.Task = TaskType(taskStr)
		p.Status = ProposalStatus(statusStr)
		p.Content = rawContent
		_ = json.Unmarshal(rawValidation, &p.Validation)

		if decidedAt.Valid {
			p.DecidedAt = &decidedAt.Time
		}
		if decidedBy.Valid {
			p.DecidedBy = &decidedBy.String
		}
		list = append(list, &p)
	}

	if list == nil {
		list = []*Proposal{}
	}
	return list, nil
}

func (s *PostgresStore) UpdateProposalStatus(ctx context.Context, tenantID, id string, status ProposalStatus, decidedBy string) (*Proposal, error) {
	query := `
		UPDATE ai_proposals
		SET status = $1, decided_at = NOW(), decided_by = $2
		WHERE tenant_id = $3 AND id = $4 AND status = 'pending_approval'
		RETURNING id, tenant_id, task, status, prompt, explanation,
		          content, validation, model_used, tokens_used, created_at,
		          decided_at, decided_by;
	`
	row := s.db.QueryRowContext(ctx, query, string(status), decidedBy, tenantID, id)

	var p Proposal
	var taskStr, statusStr string
	var rawContent, rawValidation []byte
	var decAt sql.NullTime
	var decBy sql.NullString

	err := row.Scan(
		&p.ID, &p.TenantID, &taskStr, &statusStr, &p.Prompt, &p.Explanation,
		&rawContent, &rawValidation, &p.ModelUsed, &p.TokensUsed, &p.CreatedAt,
		&decAt, &decBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// Check if it was already decided
		if existing, getErr := s.GetProposal(ctx, tenantID, id); getErr == nil {
			if existing.Status != StatusPendingApproval {
				return nil, ErrProposalDecided
			}
		}
		return nil, ErrProposalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update proposal status: %w", err)
	}

	p.Task = TaskType(taskStr)
	p.Status = ProposalStatus(statusStr)
	p.Content = rawContent
	_ = json.Unmarshal(rawValidation, &p.Validation)
	if decAt.Valid {
		p.DecidedAt = &decAt.Time
	}
	if decBy.Valid {
		p.DecidedBy = &decBy.String
	}

	return &p, nil
}
