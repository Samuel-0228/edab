package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PostgresDeadLetterStore manages dead-letter failure records in PostgreSQL.
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
