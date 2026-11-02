package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const execColumns = `id, workflow_id, event_id, status, attempts, max_attempts, next_attempt_at, started_at, completed_at, duration_ms, error, created_at, updated_at, locked_by, lease_expires_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanExecution(row rowScanner) (*Execution, error) {
	var exec Execution
	var lockedBy sql.NullString
	var leaseExpiresAt sql.NullTime
	err := row.Scan(
		&exec.ID,
		&exec.WorkflowID,
		&exec.EventID,
		&exec.Status,
		&exec.Attempts,
		&exec.MaxAttempts,
		&exec.NextAttemptAt,
		&exec.StartedAt,
		&exec.CompletedAt,
		&exec.DurationMs,
		&exec.Error,
		&exec.CreatedAt,
		&exec.UpdatedAt,
		&lockedBy,
		&leaseExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	if lockedBy.Valid {
		exec.LockedBy = &lockedBy.String
	}
	if leaseExpiresAt.Valid {
		t := leaseExpiresAt.Time
		exec.LeaseExpiresAt = &t
	}
	return &exec, nil
}

// PostgresExecutionStore manages execution history in PostgreSQL.
type PostgresExecutionStore struct {
	db *sql.DB
}

func NewPostgresExecutionStore(db *sql.DB) *PostgresExecutionStore {
	return &PostgresExecutionStore{db: db}
}

func (s *PostgresExecutionStore) Create(ctx context.Context, exec *Execution) (*Execution, error) {
	if exec.ID == "" {
		exec.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	exec.CreatedAt = now
	exec.UpdatedAt = now
	exec.StartedAt = now
	if exec.Status == "" {
		exec.Status = ExecutionPending
	}
	if exec.MaxAttempts <= 0 {
		exec.MaxAttempts = 3
	}
	if exec.NextAttemptAt.IsZero() {
		exec.NextAttemptAt = now
	}

	query := `
		INSERT INTO executions (id, workflow_id, event_id, status, attempts, max_attempts, next_attempt_at, started_at, duration_ms, error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (workflow_id, event_id) DO UPDATE SET updated_at = NOW()
		RETURNING ` + execColumns + `
	`
	created, err := scanExecution(s.db.QueryRowContext(ctx, query,
		exec.ID,
		exec.WorkflowID,
		exec.EventID,
		exec.Status,
		exec.Attempts,
		exec.MaxAttempts,
		exec.NextAttemptAt,
		exec.StartedAt,
		exec.DurationMs,
		exec.Error,
		exec.CreatedAt,
		exec.UpdatedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("failed to insert execution: %w", err)
	}

	return created, nil
}

func (s *PostgresExecutionStore) GetByID(ctx context.Context, id string) (*Execution, error) {
	query := `
		SELECT ` + execColumns + `
		FROM executions
		WHERE id = $1
	`
	exec, err := scanExecution(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	steps, _ := s.getExecutionSteps(ctx, exec.ID)
	exec.Steps = steps
	return exec, nil
}

func (s *PostgresExecutionStore) GetByWorkflowAndEvent(ctx context.Context, workflowID, eventID string) (*Execution, error) {
	query := `
		SELECT ` + execColumns + `
		FROM executions
		WHERE workflow_id = $1 AND event_id = $2
	`
	exec, err := scanExecution(s.db.QueryRowContext(ctx, query, workflowID, eventID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	steps, _ := s.getExecutionSteps(ctx, exec.ID)
	exec.Steps = steps
	return exec, nil
}

func (s *PostgresExecutionStore) List(ctx context.Context, filter ExecutionFilter) ([]*Execution, error) {
	query := `
		SELECT ` + execColumns + `
		FROM executions
		WHERE ($1 = '' OR workflow_id = $1)
		  AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, query, filter.WorkflowID, string(filter.Status), limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Execution
	for rows.Next() {
		exec, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan execution row: %w", err)
		}
		list = append(list, exec)
	}
	if list == nil {
		list = []*Execution{}
	}
	return list, nil
}

// ClaimPendingWithLease claims the next pending, waiting, retrying, or expired running execution using FOR UPDATE SKIP LOCKED.
func (s *PostgresExecutionStore) ClaimPendingWithLease(ctx context.Context, workerID string, leaseDuration time.Duration) (*Execution, error) {
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	leaseSeconds := int64(leaseDuration.Seconds())
	if leaseSeconds <= 0 {
		leaseSeconds = 1
	}

	query := `
		UPDATE executions
		SET status = 'running',
		    locked_by = $1,
		    lease_expires_at = NOW() + ($2 || ' seconds')::interval,
		    updated_at = NOW()
		WHERE id = (
			SELECT id FROM executions
			WHERE (
				((status IN ('pending', 'retrying', 'waiting')) AND next_attempt_at <= NOW())
				OR
				(status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at <= NOW())
			)
			ORDER BY priority DESC, next_attempt_at ASC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING ` + execColumns + `;
	`
	exec, err := scanExecution(s.db.QueryRowContext(ctx, query, workerID, leaseSeconds))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending execution with lease: %w", err)
	}

	steps, _ := s.getExecutionSteps(ctx, exec.ID)
	exec.Steps = steps
	return exec, nil
}

// ClaimPending delegates to ClaimPendingWithLease using a default 30-second lease.
func (s *PostgresExecutionStore) ClaimPending(ctx context.Context) (*Execution, error) {
	return s.ClaimPendingWithLease(ctx, "default-worker", 30*time.Second)
}

// RenewLease extends the lease duration of an actively running execution for workerID.
func (s *PostgresExecutionStore) RenewLease(ctx context.Context, id string, workerID string, leaseDuration time.Duration) error {
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	leaseSeconds := int64(leaseDuration.Seconds())
	if leaseSeconds <= 0 {
		leaseSeconds = 1
	}

	query := `
		UPDATE executions
		SET lease_expires_at = NOW() + ($3 || ' seconds')::interval,
		    updated_at = NOW()
		WHERE id = $1 AND locked_by = $2 AND status = 'running';
	`
	res, err := s.db.ExecContext(ctx, query, id, workerID, leaseSeconds)
	if err != nil {
		return fmt.Errorf("failed to renew execution lease: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return errors.New("execution lease lost or not running")
	}
	return nil
}

// ReleaseLease unlocks a running execution immediately back to pending.
func (s *PostgresExecutionStore) ReleaseLease(ctx context.Context, id string, workerID string) error {
	query := `
		UPDATE executions
		SET status = 'pending',
		    locked_by = NULL,
		    lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND locked_by = $2 AND status = 'running';
	`
	_, err := s.db.ExecContext(ctx, query, id, workerID)
	if err != nil {
		return fmt.Errorf("failed to release execution lease: %w", err)
	}
	return nil
}

func (s *PostgresExecutionStore) UpdateStatus(ctx context.Context, id string, status ExecutionStatus, errStr *string, durationMs int64) error {
	var query string
	var args []any
	now := time.Now().UTC()

	if status == ExecutionSuccess || status == ExecutionFailed || status == ExecutionCancelled {
		query = `
			UPDATE executions
			SET status = $1, error = $2, duration_ms = $3, completed_at = $4, locked_by = NULL, lease_expires_at = NULL, updated_at = $5
			WHERE id = $6
		`
		args = []any{status, errStr, durationMs, now, now, id}
	} else if status == ExecutionWaiting || status == ExecutionRetrying {
		query = `
			UPDATE executions
			SET status = $1, error = $2, duration_ms = $3, locked_by = NULL, lease_expires_at = NULL, updated_at = $4
			WHERE id = $5
		`
		args = []any{status, errStr, durationMs, now, id}
	} else {
		query = `
			UPDATE executions
			SET status = $1, error = $2, duration_ms = $3, updated_at = $4
			WHERE id = $5
		`
		args = []any{status, errStr, durationMs, now, id}
	}

	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *PostgresExecutionStore) RecordStep(ctx context.Context, step *ExecutionStep) error {
	if step.ID == "" {
		step.ID = uuid.NewString()
	}
	inJSON, outJSON := marshalStepIO(step.Input, step.Output)

	query := `
		INSERT INTO execution_steps (id, execution_id, step_type, step_id, name, status, input, output, error, duration_ms, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := s.db.ExecContext(ctx, query,
		step.ID,
		step.ExecutionID,
		step.StepType,
		step.StepID,
		step.Name,
		step.Status,
		inJSON,
		outJSON,
		step.Error,
		step.DurationMs,
		step.StartedAt,
		step.CompletedAt,
	)
	return err
}

func (s *PostgresExecutionStore) ResetForRetry(ctx context.Context, id string) (*Execution, error) {
	query := `
		UPDATE executions
		SET status = 'pending', next_attempt_at = NOW(), error = NULL, locked_by = NULL, lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $1
		RETURNING ` + execColumns + `;
	`
	exec, err := scanExecution(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExecutionNotFound
	}
	if err != nil {
		return nil, err
	}
	return exec, nil
}

func (s *PostgresExecutionStore) getExecutionSteps(ctx context.Context, executionID string) ([]ExecutionStep, error) {
	query := `
		SELECT id, execution_id, step_type, COALESCE(step_id, ''), name, status, input, output, error, duration_ms, started_at, completed_at
		FROM execution_steps
		WHERE execution_id = $1
		ORDER BY started_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query, executionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var steps []ExecutionStep
	for rows.Next() {
		var step ExecutionStep
		var inJSON, outJSON []byte
		if err := rows.Scan(
			&step.ID,
			&step.ExecutionID,
			&step.StepType,
			&step.StepID,
			&step.Name,
			&step.Status,
			&inJSON,
			&outJSON,
			&step.Error,
			&step.DurationMs,
			&step.StartedAt,
			&step.CompletedAt,
		); err != nil {
			return nil, err
		}
		unmarshalStepIO(&step, inJSON, outJSON)
		steps = append(steps, step)
	}
	return steps, nil
}
