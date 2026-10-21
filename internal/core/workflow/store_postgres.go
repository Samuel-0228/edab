package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"kerob/internal/core/conditions"
	"kerob/internal/core/transformations"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// Workflows PostgreSQL Store

func (s *PostgresStore) Create(ctx context.Context, wf *Workflow) (*Workflow, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}
	if wf.ID == "" {
		wf.ID = uuid.NewString()
	}
	if wf.MaxRetries <= 0 {
		wf.MaxRetries = 3
	}
	if wf.TimeoutSeconds <= 0 {
		wf.TimeoutSeconds = 30
	}
	now := time.Now().UTC()
	wf.CreatedAt = now
	wf.UpdatedAt = now

	for i := range wf.Actions {
		if wf.Actions[i].ID == "" {
			wf.Actions[i].ID = uuid.NewString()
		}
	}

	trigJSON, err := json.Marshal(wf.Trigger)
	if err != nil {
		return nil, err
	}
	condJSON, err := json.Marshal(wf.Conditions)
	if err != nil {
		return nil, err
	}
	transJSON, err := json.Marshal(wf.Transformations)
	if err != nil {
		return nil, err
	}
	actsJSON, err := json.Marshal(wf.Actions)
	if err != nil {
		return nil, err
	}

	query := `
		INSERT INTO workflows (id, name, description, enabled, trigger_type, trigger_config, conditions, transformations, actions, max_retries, timeout_seconds, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err = s.db.ExecContext(ctx, query,
		wf.ID,
		wf.Name,
		wf.Description,
		wf.Enabled,
		wf.Trigger.Type,
		trigJSON,
		condJSON,
		transJSON,
		actsJSON,
		wf.MaxRetries,
		wf.TimeoutSeconds,
		wf.CreatedAt,
		wf.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert workflow: %w", err)
	}

	return wf, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Workflow, error) {
	query := `
		SELECT id, name, description, enabled, trigger_config, conditions, transformations, actions, max_retries, timeout_seconds, created_at, updated_at
		FROM workflows
		WHERE id = $1
	`
	var wf Workflow
	var trigJSON, condJSON, transJSON, actsJSON []byte

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&wf.ID,
		&wf.Name,
		&wf.Description,
		&wf.Enabled,
		&trigJSON,
		&condJSON,
		&transJSON,
		&actsJSON,
		&wf.MaxRetries,
		&wf.TimeoutSeconds,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkflowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	if len(trigJSON) > 0 {
		_ = json.Unmarshal(trigJSON, &wf.Trigger)
	}
	if len(condJSON) > 0 && string(condJSON) != "null" && string(condJSON) != "{}" {
		var c conditions.Condition
		if err := json.Unmarshal(condJSON, &c); err == nil {
			wf.Conditions = &c
		}
	}
	if len(transJSON) > 0 && string(transJSON) != "null" && string(transJSON) != "{}" {
		var t transformations.Transformation
		if err := json.Unmarshal(transJSON, &t); err == nil {
			wf.Transformations = &t
		}
	}
	if len(actsJSON) > 0 {
		_ = json.Unmarshal(actsJSON, &wf.Actions)
	}

	return &wf, nil
}

func (s *PostgresStore) List(ctx context.Context) ([]*Workflow, error) {
	return s.queryList(ctx, `SELECT id, name, description, enabled, trigger_config, conditions, transformations, actions, max_retries, timeout_seconds, created_at, updated_at FROM workflows ORDER BY created_at DESC`)
}

func (s *PostgresStore) ListActive(ctx context.Context) ([]*Workflow, error) {
	return s.queryList(ctx, `SELECT id, name, description, enabled, trigger_config, conditions, transformations, actions, max_retries, timeout_seconds, created_at, updated_at FROM workflows WHERE enabled = true ORDER BY created_at DESC`)
}

func (s *PostgresStore) queryList(ctx context.Context, query string) ([]*Workflow, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Workflow
	for rows.Next() {
		var wf Workflow
		var trigJSON, condJSON, transJSON, actsJSON []byte
		if err := rows.Scan(
			&wf.ID,
			&wf.Name,
			&wf.Description,
			&wf.Enabled,
			&trigJSON,
			&condJSON,
			&transJSON,
			&actsJSON,
			&wf.MaxRetries,
			&wf.TimeoutSeconds,
			&wf.CreatedAt,
			&wf.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan workflow row: %w", err)
		}

		if len(trigJSON) > 0 {
			_ = json.Unmarshal(trigJSON, &wf.Trigger)
		}
		if len(condJSON) > 0 && string(condJSON) != "null" && string(condJSON) != "{}" {
			var c conditions.Condition
			if err := json.Unmarshal(condJSON, &c); err == nil {
				wf.Conditions = &c
			}
		}
		if len(transJSON) > 0 && string(transJSON) != "null" && string(transJSON) != "{}" {
			var t transformations.Transformation
			if err := json.Unmarshal(transJSON, &t); err == nil {
				wf.Transformations = &t
			}
		}
		if len(actsJSON) > 0 {
			_ = json.Unmarshal(actsJSON, &wf.Actions)
		}

		list = append(list, &wf)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*Workflow{}
	}
	return list, nil
}

func (s *PostgresStore) Update(ctx context.Context, wf *Workflow) (*Workflow, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}
	wf.UpdatedAt = time.Now().UTC()

	trigJSON, err := json.Marshal(wf.Trigger)
	if err != nil {
		return nil, err
	}
	condJSON, err := json.Marshal(wf.Conditions)
	if err != nil {
		return nil, err
	}
	transJSON, err := json.Marshal(wf.Transformations)
	if err != nil {
		return nil, err
	}
	actsJSON, err := json.Marshal(wf.Actions)
	if err != nil {
		return nil, err
	}

	query := `
		UPDATE workflows
		SET name = $1, description = $2, enabled = $3, trigger_type = $4, trigger_config = $5,
		    conditions = $6, transformations = $7, actions = $8, max_retries = $9, timeout_seconds = $10, updated_at = $11
		WHERE id = $12
	`
	res, err := s.db.ExecContext(ctx, query,
		wf.Name,
		wf.Description,
		wf.Enabled,
		wf.Trigger.Type,
		trigJSON,
		condJSON,
		transJSON,
		actsJSON,
		wf.MaxRetries,
		wf.TimeoutSeconds,
		wf.UpdatedAt,
		wf.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, ErrWorkflowNotFound
	}
	return wf, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM workflows WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrWorkflowNotFound
	}
	return nil
}

// Executions PostgreSQL Store

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
	inJSON, _ := json.Marshal(step.Input)
	outJSON, _ := json.Marshal(step.Output)

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
		if len(inJSON) > 0 {
			_ = json.Unmarshal(inJSON, &step.Input)
		}
		if len(outJSON) > 0 {
			_ = json.Unmarshal(outJSON, &step.Output)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// DeadLetter PostgreSQL Store

type PostgresDeadLetterStore struct {
	db *sql.DB
}

func NewPostgresDeadLetterStore(db *sql.DB) *PostgresDeadLetterStore {
	return &PostgresDeadLetterStore{db: db}
}

func (s *PostgresDeadLetterStore) Record(ctx context.Context, rec *DeadLetterRecord) (*DeadLetterRecord, error) {
	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	if rec.FailedAt.IsZero() {
		rec.FailedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO dead_letter (id, execution_id, workflow_id, event_id, action_id, error, payload, attempts, discarded, failed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := s.db.ExecContext(ctx, query,
		rec.ID,
		rec.ExecutionID,
		rec.WorkflowID,
		rec.EventID,
		rec.ActionID,
		rec.Error,
		rec.Payload,
		rec.Attempts,
		rec.Discarded,
		rec.FailedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert dead letter: %w", err)
	}
	return rec, nil
}

func (s *PostgresDeadLetterStore) GetByID(ctx context.Context, id string) (*DeadLetterRecord, error) {
	query := `
		SELECT id, execution_id, workflow_id, event_id, action_id, error, payload, attempts, discarded, failed_at
		FROM dead_letter
		WHERE id = $1
	`
	var rec DeadLetterRecord
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&rec.ID,
		&rec.ExecutionID,
		&rec.WorkflowID,
		&rec.EventID,
		&rec.ActionID,
		&rec.Error,
		&rec.Payload,
		&rec.Attempts,
		&rec.Discarded,
		&rec.FailedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeadLetterNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dead letter: %w", err)
	}
	return &rec, nil
}

func (s *PostgresDeadLetterStore) List(ctx context.Context, includeDiscarded bool) ([]*DeadLetterRecord, error) {
	query := `
		SELECT id, execution_id, workflow_id, event_id, action_id, error, payload, attempts, discarded, failed_at
		FROM dead_letter
		WHERE ($1 = true OR discarded = false)
		ORDER BY failed_at DESC
		LIMIT 100
	`
	rows, err := s.db.QueryContext(ctx, query, includeDiscarded)
	if err != nil {
		return nil, fmt.Errorf("failed to query dead letters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*DeadLetterRecord
	for rows.Next() {
		var rec DeadLetterRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.ExecutionID,
			&rec.WorkflowID,
			&rec.EventID,
			&rec.ActionID,
			&rec.Error,
			&rec.Payload,
			&rec.Attempts,
			&rec.Discarded,
			&rec.FailedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan dead letter row: %w", err)
		}
		list = append(list, &rec)
	}
	if list == nil {
		list = []*DeadLetterRecord{}
	}
	return list, nil
}

func (s *PostgresDeadLetterStore) Discard(ctx context.Context, id string) error {
	query := `UPDATE dead_letter SET discarded = true WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to discard dead letter: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrDeadLetterNotFound
	}
	return nil
}
