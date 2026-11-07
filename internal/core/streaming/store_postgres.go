package streaming

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PostgresOffsetStore manages stream consumer offsets in PostgreSQL.
type PostgresOffsetStore struct {
	db *sql.DB
}

// NewPostgresOffsetStore creates a new PostgresOffsetStore.
func NewPostgresOffsetStore(db *sql.DB) *PostgresOffsetStore {
	return &PostgresOffsetStore{db: db}
}

func (s *PostgresOffsetStore) CommitOffset(ctx context.Context, offset ConsumerOffset) error {
	query := `
		INSERT INTO stream_consumer_offsets (group_id, topic, partition, offset_val, committed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (group_id, topic, partition)
		DO UPDATE SET offset_val = EXCLUDED.offset_val, committed_at = EXCLUDED.committed_at;
	`
	if offset.CommittedAt.IsZero() {
		offset.CommittedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, query, offset.GroupID, offset.Topic, offset.Partition, offset.Offset, offset.CommittedAt)
	return err
}

func (s *PostgresOffsetStore) GetOffset(ctx context.Context, groupID, topic string, partition int32) (int64, error) {
	query := `
		SELECT offset_val
		FROM stream_consumer_offsets
		WHERE group_id = $1 AND topic = $2 AND partition = $3;
	`
	var off int64
	err := s.db.QueryRowContext(ctx, query, groupID, topic, partition).Scan(&off)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return off, err
}

func (s *PostgresOffsetStore) ListOffsets(ctx context.Context, groupID string) ([]ConsumerOffset, error) {
	query := `
		SELECT group_id, topic, partition, offset_val, committed_at
		FROM stream_consumer_offsets
		WHERE ($1 = '' OR group_id = $1)
		ORDER BY topic, partition;
	`
	rows, err := s.db.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []ConsumerOffset
	for rows.Next() {
		var o ConsumerOffset
		if err := rows.Scan(&o.GroupID, &o.Topic, &o.Partition, &o.Offset, &o.CommittedAt); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, nil
}

// PostgresDeadLetterStore manages dead-letter stream records in PostgreSQL.
type PostgresDeadLetterStore struct {
	db *sql.DB
}

// NewPostgresDeadLetterStore creates a new PostgresDeadLetterStore.
func NewPostgresDeadLetterStore(db *sql.DB) *PostgresDeadLetterStore {
	return &PostgresDeadLetterStore{db: db}
}

func (s *PostgresDeadLetterStore) Store(ctx context.Context, dlq *DeadLetterEvent) (*DeadLetterEvent, error) {
	if dlq.ID == "" {
		dlq.ID = uuid.NewString()
	}
	if dlq.CreatedAt.IsZero() {
		dlq.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO dead_letter_events (id, tenant_id, topic, partition, offset_val, event_id, ordering_key, event_type, payload, error_reason, retry_count, resolved, created_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, COALESCE(tenant_id, ''), topic, partition, offset_val, COALESCE(event_id, ''), COALESCE(ordering_key, ''), COALESCE(event_type, ''), payload, error_reason, retry_count, resolved, created_at;
	`

	var stored DeadLetterEvent
	err := s.db.QueryRowContext(ctx, query,
		dlq.ID, dlq.TenantID, dlq.Topic, dlq.Partition, dlq.Offset,
		dlq.EventID, dlq.OrderingKey, dlq.EventType, dlq.Payload, dlq.ErrorReason,
		dlq.RetryCount, dlq.Resolved, dlq.CreatedAt,
	).Scan(
		&stored.ID, &stored.TenantID, &stored.Topic, &stored.Partition, &stored.Offset,
		&stored.EventID, &stored.OrderingKey, &stored.EventType, &stored.Payload, &stored.ErrorReason,
		&stored.RetryCount, &stored.Resolved, &stored.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert dead letter event: %w", err)
	}
	return &stored, nil
}

func (s *PostgresDeadLetterStore) GetByID(ctx context.Context, id string) (*DeadLetterEvent, error) {
	query := `
		SELECT id, COALESCE(tenant_id, ''), topic, partition, offset_val, COALESCE(event_id, ''), COALESCE(ordering_key, ''), COALESCE(event_type, ''), payload, error_reason, retry_count, resolved, created_at
		FROM dead_letter_events
		WHERE id = $1;
	`
	var stored DeadLetterEvent
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&stored.ID, &stored.TenantID, &stored.Topic, &stored.Partition, &stored.Offset,
		&stored.EventID, &stored.OrderingKey, &stored.EventType, &stored.Payload, &stored.ErrorReason,
		&stored.RetryCount, &stored.Resolved, &stored.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDLQNotFound
	}
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

func (s *PostgresDeadLetterStore) List(ctx context.Context, filter DLQFilter) ([]*DeadLetterEvent, error) {
	query := `
		SELECT id, COALESCE(tenant_id, ''), topic, partition, offset_val, COALESCE(event_id, ''), COALESCE(ordering_key, ''), COALESCE(event_type, ''), payload, error_reason, retry_count, resolved, created_at
		FROM dead_letter_events
		WHERE ($1 = '' OR tenant_id IS NULL OR tenant_id = $1)
		  AND ($2 = '' OR topic = $2)
		  AND ($3::boolean IS NULL OR resolved = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5;
	`
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	var resolvedParam any
	if filter.Resolved != nil {
		resolvedParam = *filter.Resolved
	}

	rows, err := s.db.QueryContext(ctx, query, filter.TenantID, filter.Topic, resolvedParam, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*DeadLetterEvent
	for rows.Next() {
		var stored DeadLetterEvent
		if err := rows.Scan(
			&stored.ID, &stored.TenantID, &stored.Topic, &stored.Partition, &stored.Offset,
			&stored.EventID, &stored.OrderingKey, &stored.EventType, &stored.Payload, &stored.ErrorReason,
			&stored.RetryCount, &stored.Resolved, &stored.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, &stored)
	}
	return result, nil
}

func (s *PostgresDeadLetterStore) MarkResolved(ctx context.Context, id string) error {
	query := `UPDATE dead_letter_events SET resolved = true WHERE id = $1;`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrDLQNotFound
	}
	return nil
}

func (s *PostgresDeadLetterStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM dead_letter_events WHERE id = $1;`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrDLQNotFound
	}
	return nil
}
