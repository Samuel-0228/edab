package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

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
		RETURNING id, status, attempts, max_attempts, next_attempt_at, started_at, completed_at, duration_ms, error, created_at, updated_at
	`
	err := s.db.QueryRowContext(ctx, query,
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
	).Scan(
		&exec.ID,
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
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert execution: %w", err)
	}

	return exec, nil
}

func (s *PostgresExecutionStore) GetByID(ctx context.Context, id string) (*Execution, error) {
	query := `
		SELECT id, workflow_id, event_id, status, attempts, max_attempts, next_attempt_at, started_at, completed_at, duration_ms, error, created_at, updated_at
		FROM executions
		WHERE id = $1
	`
	var exec Execution
	err := s.db.QueryRowContext(ctx, query, id).Scan(
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
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	steps, _ := s.getExecutionSteps(ctx, exec.ID)
	exec.Steps = steps
	return &exec, nil
}

func (s *PostgresExecutionStore) GetByWorkflowAndEvent(ctx context.Context, workflowID, eventID string) (*Execution, error) {
	query := `
		SELECT id, workflow_id, event_id, status, attempts, max_attempts, next_attempt_at, started_at, completed_at, duration_ms, error, created_at, updated_at
		FROM executions
		WHERE workflow_id = $1 AND event_id = $2
	`
	var exec Execution
	err := s.db.QueryRowContext(ctx, query, workflowID, eventID).Scan(
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
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExecutionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	steps, _ := s.getExecutionSteps(ctx, exec.ID)
	exec.Steps = steps
	return &exec, nil
}

func (s *PostgresExecutionStore) List(ctx context.Context, filter ExecutionFilter) ([]*Execution, error) {
	query := `
		SELECT id, workflow_id, event_id, status, attempts, max_attempts, next_attempt_at, started_at, completed_at, duration_ms, error, created_at, updated_at
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
		var exec Execution
		if err := rows.Scan(
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
		); err != nil {
			return nil, fmt.Errorf("failed to scan execution row: %w", err)
		}
		list = append(list, &exec)
	}
	if list == nil {
		list = []*Execution{}
	}
	return list, nil
}

func (s *PostgresExecutionStore) ClaimPending(ctx context.Context) (*Execution, error) {
	query := `
		UPDATE executions
		SET status = 'running', attempts = attempts + 1, updated_at = NOW()
		WHERE id = (
			SELECT id FROM executions
			WHERE (status = 'pending' OR status = 'retrying')
			  AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, workflow_id, event_id, status, attempts, max_attempts, next_attempt_at, started_at, completed_at, duration_ms, error, created_at, updated_at
	`
	var exec Execution
	err := s.db.QueryRowContext(ctx, query).Scan(
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
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim pending execution: %w", err)
	}
	return &exec, nil
}

func (s *PostgresExecutionStore) UpdateStatus(ctx context.Context, id string, status ExecutionStatus, errStr *string, durationMs int64) error {
	var query string
	var args []any
	now := time.Now().UTC()

	if status == ExecutionSuccess || status == ExecutionFailed || status == ExecutionCancelled {
		query = `
			UPDATE executions
			SET status = $1, error = $2, duration_ms = $3, completed_at = $4, updated_at = $5
			WHERE id = $6
		`
		args = []any{status, errStr, durationMs, now, now, id}
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
		INSERT INTO execution_steps (id, execution_id, step_type, name, status, input, output, error, duration_ms, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := s.db.ExecContext(ctx, query,
		step.ID,
		step.ExecutionID,
		step.StepType,
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
		SET status = 'pending', next_attempt_at = NOW(), error = NULL, updated_at = NOW()
		WHERE id = $1
		RETURNING id, workflow_id, event_id, status, attempts, max_attempts, next_attempt_at, started_at, completed_at, duration_ms, error, created_at, updated_at
	`
	var exec Execution
	err := s.db.QueryRowContext(ctx, query, id).Scan(
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
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrExecutionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &exec, nil
}

func (s *PostgresExecutionStore) getExecutionSteps(ctx context.Context, executionID string) ([]ExecutionStep, error) {
	query := `
		SELECT id, execution_id, step_type, name, status, input, output, error, duration_ms, started_at, completed_at
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
