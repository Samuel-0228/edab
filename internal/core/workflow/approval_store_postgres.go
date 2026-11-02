package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const approvalColumns = `
	id, tenant_id, execution_id, step_id, workflow_id, title, description, token, token_hash,
	status, approver_roles, approver_users, approved_by, decision_note, expires_at, timeout_action,
	escalation_roles, escalation_contact, created_at, updated_at
`

// PostgresApprovalStore manages human approval persistence in PostgreSQL.
type PostgresApprovalStore struct {
	db *sql.DB
}

// NewPostgresApprovalStore initializes a Postgres-backed approval store.
func NewPostgresApprovalStore(db *sql.DB) *PostgresApprovalStore {
	return &PostgresApprovalStore{db: db}
}

func scanApproval(row rowScanner) (*Approval, error) {
	var app Approval
	var desc, approvedBy, decisionNote sql.NullString
	var approverRolesJSON, approverUsersJSON, escalationRolesJSON []byte

	err := row.Scan(
		&app.ID,
		&app.TenantID,
		&app.ExecutionID,
		&app.StepID,
		&app.WorkflowID,
		&app.Title,
		&desc,
		&app.Token,
		&app.TokenHash,
		&app.Status,
		&approverRolesJSON,
		&approverUsersJSON,
		&approvedBy,
		&decisionNote,
		&app.ExpiresAt,
		&app.TimeoutAction,
		&escalationRolesJSON,
		&app.EscalationContact,
		&app.CreatedAt,
		&app.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if desc.Valid {
		app.Description = desc.String
	}
	if approvedBy.Valid {
		app.ApprovedBy = &approvedBy.String
	}
	if decisionNote.Valid {
		app.DecisionNote = &decisionNote.String
	}

	if len(approverRolesJSON) > 0 {
		_ = json.Unmarshal(approverRolesJSON, &app.ApproverRoles)
	}
	if len(approverUsersJSON) > 0 {
		_ = json.Unmarshal(approverUsersJSON, &app.ApproverUsers)
	}
	if len(escalationRolesJSON) > 0 {
		_ = json.Unmarshal(escalationRolesJSON, &app.EscalationRoles)
	}

	return &app, nil
}

func (s *PostgresApprovalStore) Create(ctx context.Context, app *Approval) (*Approval, error) {
	if app.ID == "" {
		app.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	app.CreatedAt = now
	app.UpdatedAt = now
	if app.Status == "" {
		app.Status = ApprovalPending
	}
	if app.ExpiresAt.IsZero() {
		app.ExpiresAt = now.Add(24 * time.Hour)
	}
	if app.TenantID == "" {
		app.TenantID = "default"
	}

	rolesBytes, _ := json.Marshal(app.ApproverRoles)
	usersBytes, _ := json.Marshal(app.ApproverUsers)
	escRolesBytes, _ := json.Marshal(app.EscalationRoles)

	query := `
		INSERT INTO workflow_approvals (
			id, tenant_id, execution_id, step_id, workflow_id, title, description, token, token_hash,
			status, approver_roles, approver_users, approved_by, decision_note, expires_at, timeout_action,
			escalation_roles, escalation_contact, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		ON CONFLICT (execution_id, step_id) DO UPDATE SET updated_at = NOW()
		RETURNING ` + approvalColumns + `;
	`

	created, err := scanApproval(s.db.QueryRowContext(ctx, query,
		app.ID, app.TenantID, app.ExecutionID, app.StepID, app.WorkflowID, app.Title, app.Description,
		app.Token, app.TokenHash, string(app.Status), rolesBytes, usersBytes, app.ApprovedBy, app.DecisionNote,
		app.ExpiresAt, app.TimeoutAction, escRolesBytes, app.EscalationContact, app.CreatedAt, app.UpdatedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to create approval record: %w", err)
	}
	return created, nil
}

func (s *PostgresApprovalStore) GetByID(ctx context.Context, id string) (*Approval, error) {
	query := `SELECT ` + approvalColumns + ` FROM workflow_approvals WHERE id = $1;`
	app, err := scanApproval(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get approval: %w", err)
	}
	return app, nil
}

func (s *PostgresApprovalStore) GetByTokenHash(ctx context.Context, tokenHash string) (*Approval, error) {
	query := `SELECT ` + approvalColumns + ` FROM workflow_approvals WHERE token_hash = $1;`
	app, err := scanApproval(s.db.QueryRowContext(ctx, query, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get approval by token: %w", err)
	}
	return app, nil
}

func (s *PostgresApprovalStore) GetByExecutionAndStep(ctx context.Context, executionID, stepID string) (*Approval, error) {
	query := `SELECT ` + approvalColumns + ` FROM workflow_approvals WHERE execution_id = $1 AND step_id = $2;`
	app, err := scanApproval(s.db.QueryRowContext(ctx, query, executionID, stepID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get approval by execution and step: %w", err)
	}
	return app, nil
}

func (s *PostgresApprovalStore) List(ctx context.Context, filter ApprovalFilter) ([]*Approval, error) {
	query := `
		SELECT ` + approvalColumns + `
		FROM workflow_approvals
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR execution_id = $2)
		  AND ($3 = '' OR workflow_id = $3)
		  AND ($4 = '' OR status = $4)
		ORDER BY created_at DESC
		LIMIT $5 OFFSET $6;
	`
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, query, filter.TenantID, filter.ExecutionID, filter.WorkflowID, string(filter.Status), limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Approval
	for rows.Next() {
		app, err := scanApproval(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval: %w", err)
		}
		list = append(list, app)
	}
	if list == nil {
		list = []*Approval{}
	}
	return list, nil
}

func (s *PostgresApprovalStore) UpdateDecision(ctx context.Context, id string, status ApprovalStatus, decidedBy string, note string) (*Approval, error) {
	query := `
		UPDATE workflow_approvals
		SET status = $1, approved_by = $2, decision_note = $3, updated_at = NOW()
		WHERE id = $4 AND status IN ('pending', 'escalated')
		RETURNING ` + approvalColumns + `;
	`
	app, err := scanApproval(s.db.QueryRowContext(ctx, query, string(status), decidedBy, note, id))
	if errors.Is(err, sql.ErrNoRows) {
		// Check if it exists but is already closed
		existing, gErr := s.GetByID(ctx, id)
		if gErr == nil && existing != nil {
			return nil, ErrApprovalAlreadyClosed
		}
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update approval decision: %w", err)
	}
	return app, nil
}

func (s *PostgresApprovalStore) UpdateStatus(ctx context.Context, id string, status ApprovalStatus) error {
	query := `UPDATE workflow_approvals SET status = $1, updated_at = NOW() WHERE id = $2;`
	res, err := s.db.ExecContext(ctx, query, string(status), id)
	if err != nil {
		return fmt.Errorf("failed to update approval status: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrApprovalNotFound
	}
	return nil
}

func (s *PostgresApprovalStore) ListExpiredPending(ctx context.Context, now time.Time) ([]*Approval, error) {
	query := `
		SELECT ` + approvalColumns + `
		FROM workflow_approvals
		WHERE status IN ('pending', 'escalated')
		  AND expires_at <= $1
		ORDER BY expires_at ASC
		LIMIT 50;
	`
	rows, err := s.db.QueryContext(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query expired approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Approval
	for rows.Next() {
		app, err := scanApproval(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan expired approval: %w", err)
		}
		list = append(list, app)
	}
	if list == nil {
		list = []*Approval{}
	}
	return list, nil
}

func (s *PostgresApprovalStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM workflow_approvals WHERE id = $1;`
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}
