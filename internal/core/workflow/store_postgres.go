package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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

	payloads, err := marshalWorkflowComponents(wf)
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
		payloads.Trigger,
		payloads.Conditions,
		payloads.Transformations,
		payloads.Actions,
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

	unmarshalWorkflowComponents(&wf, trigJSON, condJSON, transJSON, actsJSON)
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

		unmarshalWorkflowComponents(&wf, trigJSON, condJSON, transJSON, actsJSON)
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

	payloads, err := marshalWorkflowComponents(wf)
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
		payloads.Trigger,
		payloads.Conditions,
		payloads.Transformations,
		payloads.Actions,
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
