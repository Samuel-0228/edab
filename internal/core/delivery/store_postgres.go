package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
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

const jobColumns = `id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, result, delivered_at, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (*DeliveryJob, error) {
	var job DeliveryJob
	var deliveredAt sql.NullTime
	var rawResult []byte
	err := row.Scan(
		&job.ID,
		&job.EventID,
		&job.DestinationID,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&job.NextAttemptAt,
		&job.LastError,
		&rawResult,
		&deliveredAt,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if deliveredAt.Valid {
		t := deliveredAt.Time
		job.DeliveredAt = &t
	}
	if len(rawResult) > 0 {
		job.Result = json.RawMessage(rawResult)
	}
	return &job, nil
}

func (s *PostgresStore) CreateJob(ctx context.Context, job *DeliveryJob) error {
	query := `
		INSERT INTO delivery_jobs (
			id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, result, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (event_id, destination_id) DO NOTHING;
	`
	var resultParam any
	if len(job.Result) > 0 {
		resultParam = []byte(job.Result)
	}
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
		resultParam,
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
			id, event_id, destination_id, status, attempts, max_attempts, next_attempt_at, last_error, result, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (event_id, destination_id) DO NOTHING;
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare delivery job insert: %w", err)
	}
	defer stmt.Close()

	for _, job := range jobs {
		var resultParam any
		if len(job.Result) > 0 {
			resultParam = []byte(job.Result)
		}
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
			resultParam,
			job.CreatedAt,
			job.UpdatedAt,
		); err != nil {
			return fmt.Errorf("failed to insert delivery job: %w", err)
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*DeliveryJob, error) {
	query := `SELECT ` + jobColumns + ` FROM delivery_jobs WHERE id = $1;`
	job, err := scanJob(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get delivery job: %w", err)
	}
	return job, nil
}

func (s *PostgresStore) List(ctx context.Context, filter ListFilter) ([]*DeliveryJob, error) {
	query := `
		SELECT ` + jobColumns + `
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
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan delivery job row: %w", err)
		}
		jobs = append(jobs, job)
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
		RETURNING ` + jobColumns + `;
	`

	job, err := scanJob(s.db.QueryRowContext(ctx, query))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No jobs ready to claim
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim next delivery job: %w", err)
	}
	return job, nil
}

func (s *PostgresStore) MarkDelivered(ctx context.Context, id string) error {
	return s.MarkDeliveredWithResult(ctx, id, nil)
}

func (s *PostgresStore) MarkDeliveredWithResult(ctx context.Context, id string, result json.RawMessage) error {
	var resultParam any
	if len(result) > 0 {
		resultParam = []byte(result)
	}
	query := `
		UPDATE delivery_jobs
		SET status = 'delivered', result = $2, delivered_at = NOW(), updated_at = NOW()
		WHERE id = $1;
	`
	_, err := s.db.ExecContext(ctx, query, id, resultParam)
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

// ResetForRetry resets a failed or dead-letter job for another attempt. The
// recorded result is intentionally preserved so an already-completed delivery
// is not repeated.
func (s *PostgresStore) ResetForRetry(ctx context.Context, id string) (*DeliveryJob, error) {
	query := `
		UPDATE delivery_jobs
		SET status = 'pending', attempts = 0, next_attempt_at = NOW(), last_error = NULL, updated_at = NOW()
		WHERE id = $1
		RETURNING ` + jobColumns + `;
	`
	job, err := scanJob(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to reset job for retry: %w", err)
	}
	return job, nil
}

func (s *PostgresStore) Stats(ctx context.Context) (Stats, error) {
	query := `SELECT status, COUNT(*) FROM delivery_jobs GROUP BY status;`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return Stats{}, fmt.Errorf("failed to query delivery stats: %w", err)
	}
	defer rows.Close()

	var stats Stats
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return Stats{}, fmt.Errorf("failed to scan delivery stats: %w", err)
		}
		switch Status(status) {
		case StatusPending:
			stats.Pending = count
		case StatusProcessing:
			stats.Processing = count
		case StatusDelivered:
			stats.Delivered = count
		case StatusFailed:
			stats.Failed = count
		case StatusDeadLetter:
			stats.DeadLetter = count
		}
		stats.Total += count
	}
	if err := rows.Err(); err != nil {
		return Stats{}, fmt.Errorf("rows error: %w", err)
	}
	return stats, nil
}
