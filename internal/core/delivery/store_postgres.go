package delivery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateJob(ctx context.Context, job *DeliveryJob) error {
	query := `
		INSERT INTO delivery_jobs (
			id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (event_id, destination_id) DO NOTHING;
	`
	_, err := s.db.ExecContext(
		ctx,
		query,
		job.ID,
		job.EventID,
		job.DestinationID,
		job.Status,
		job.Attempts,
		job.MaxAttempts,
		job.NextAttemptAt,
		job.LastError,
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create delivery job: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateJobs(ctx context.Context, jobs []*DeliveryJob) error {
	if len(jobs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx for delivery jobs: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO delivery_jobs (
			id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (event_id, destination_id) DO NOTHING;
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare delivery job insert: %w", err)
	}
	defer stmt.Close()

	for _, job := range jobs {
		if _, err := stmt.ExecContext(
			ctx,
			job.ID,
			job.EventID,
			job.DestinationID,
			job.Status,
			job.Attempts,
			job.MaxAttempts,
			job.NextAttemptAt,
			job.LastError,
			job.CreatedAt,
			job.UpdatedAt,
		); err != nil {
			return fmt.Errorf("failed to insert delivery job: %w", err)
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*DeliveryJob, error) {
	query := `
		SELECT id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, created_at, updated_at
		FROM delivery_jobs
		WHERE id = $1;
	`
	var job DeliveryJob
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&job.ID,
		&job.EventID,
		&job.DestinationID,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&job.NextAttemptAt,
		&job.LastError,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get delivery job: %w", err)
	}
	return &job, nil
}

func (s *PostgresStore) List(ctx context.Context, filter ListFilter) ([]*DeliveryJob, error) {
	query := `
		SELECT id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, created_at, updated_at
		FROM delivery_jobs
		WHERE ($1 = '' OR event_id = $1)
		  AND ($2 = '' OR destination_id = $2)
		  AND ($3 = '' OR status = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5;
	`
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, query, filter.EventID, filter.DestinationID, filter.Status, limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list delivery jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*DeliveryJob
	for rows.Next() {
		var job DeliveryJob
		if err := rows.Scan(
			&job.ID,
			&job.EventID,
			&job.DestinationID,
			&job.Status,
			&job.Attempts,
			&job.MaxAttempts,
			&job.NextAttemptAt,
			&job.LastError,
			&job.CreatedAt,
			&job.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan delivery job row: %w", err)
		}
		jobs = append(jobs, &job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return jobs, nil
}

// ClaimNextJob safely claims the next pending or retryable job using FOR UPDATE SKIP LOCKED.
func (s *PostgresStore) ClaimNextJob(ctx context.Context) (*DeliveryJob, error) {
	query := `
		UPDATE delivery_jobs
		SET status = 'processing',
		    attempts = attempts + 1,
		    updated_at = NOW()
		WHERE id = (
			SELECT id
			FROM delivery_jobs
			WHERE status IN ('pending', 'failed')
			  AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at ASC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, created_at, updated_at;
	`

	var job DeliveryJob
	err := s.db.QueryRowContext(ctx, query).Scan(
		&job.ID,
		&job.EventID,
		&job.DestinationID,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&job.NextAttemptAt,
		&job.LastError,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No jobs ready to claim
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim next delivery job: %w", err)
	}
	return &job, nil
}

func (s *PostgresStore) MarkDelivered(ctx context.Context, id string) error {
	query := `
		UPDATE delivery_jobs
		SET status = 'delivered', updated_at = NOW()
		WHERE id = $1;
	`
	_, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark job delivered: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkFailed(ctx context.Context, id string, errStr string, nextAttempt time.Time) error {
	query := `
		UPDATE delivery_jobs
		SET status = 'failed', last_error = $2, next_attempt_at = $3, updated_at = NOW()
		WHERE id = $1;
	`
	_, err := s.db.ExecContext(ctx, query, id, errStr, nextAttempt)
	if err != nil {
		return fmt.Errorf("failed to mark job failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkDeadLetter(ctx context.Context, id string, errStr string) error {
	query := `
		UPDATE delivery_jobs
		SET status = 'dead_letter', last_error = $2, updated_at = NOW()
		WHERE id = $1;
	`
	_, err := s.db.ExecContext(ctx, query, id, errStr)
	if err != nil {
		return fmt.Errorf("failed to mark job dead_letter: %w", err)
	}
	return nil
}

func (s *PostgresStore) ResetForRetry(ctx context.Context, id string) (*DeliveryJob, error) {
	query := `
		UPDATE delivery_jobs
		SET status = 'pending', attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_at = NOW()
		WHERE id = $1
		RETURNING id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, created_at, updated_at;
	`
	var job DeliveryJob
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&job.ID,
		&job.EventID,
		&job.DestinationID,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&job.NextAttemptAt,
		&job.LastError,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to reset job for retry: %w", err)
	}
	return &job, nil
}
