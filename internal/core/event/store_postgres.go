package event

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Create(ctx context.Context, e *Event) (*Event, bool, error) {
	query := `
		INSERT INTO events (id, source_id, external_event_id, event_type, payload, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (source_id, external_event_id) DO NOTHING
		RETURNING id, source_id, external_event_id, event_type, payload, metadata, created_at;
	`

	var created Event
	err := s.db.QueryRowContext(
		ctx,
		query,
		e.ID,
		e.SourceID,
		e.ExternalEventID,
		e.EventType,
		e.Payload,
		e.Metadata,
		e.CreatedAt,
	).Scan(
		&created.ID,
		&created.SourceID,
		&created.ExternalEventID,
		&created.EventType,
		&created.Payload,
		&created.Metadata,
		&created.CreatedAt,
	)

	if err == nil {
		return &created, true, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Event already exists (idempotency triggered)
		existing, err := s.GetByExternalID(ctx, e.SourceID, e.ExternalEventID)
		if err != nil {
			return nil, false, fmt.Errorf("failed to load existing event on conflict: %w", err)
		}
		return existing, false, nil
	}

	return nil, false, fmt.Errorf("failed to insert event: %w", err)
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Event, error) {
	query := `
		SELECT id, source_id, external_event_id, event_type, payload, metadata, created_at
		FROM events
		WHERE id = $1;
	`
	var e Event
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&e.ID,
		&e.SourceID,
		&e.ExternalEventID,
		&e.EventType,
		&e.Payload,
		&e.Metadata,
		&e.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get event by id: %w", err)
	}
	return &e, nil
}

func (s *PostgresStore) GetByExternalID(ctx context.Context, sourceID, externalID string) (*Event, error) {
	query := `
		SELECT id, source_id, external_event_id, event_type, payload, metadata, created_at
		FROM events
		WHERE source_id = $1 AND external_event_id = $2;
	`
	var e Event
	err := s.db.QueryRowContext(ctx, query, sourceID, externalID).Scan(
		&e.ID,
		&e.SourceID,
		&e.ExternalEventID,
		&e.EventType,
		&e.Payload,
		&e.Metadata,
		&e.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get event by external id: %w", err)
	}
	return &e, nil
}

func (s *PostgresStore) List(ctx context.Context, filter ListFilter) ([]*Event, error) {
	query := `
		SELECT id, source_id, external_event_id, event_type, payload, metadata, created_at
		FROM events
		WHERE ($1 = '' OR source_id = $1)
		  AND ($2 = '' OR event_type = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4;
	`
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, query, filter.SourceID, filter.EventType, limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.ID,
			&e.SourceID,
			&e.ExternalEventID,
			&e.EventType,
			&e.Payload,
			&e.Metadata,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}
		events = append(events, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return events, nil
}
