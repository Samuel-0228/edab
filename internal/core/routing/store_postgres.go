package routing

import (
	"context"
	"database/sql"
	"encoding/json"
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

func (s *PostgresStore) Create(ctx context.Context, r *Route) (*Route, error) {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	if len(r.Filters) == 0 {
		r.Filters = json.RawMessage("{}")
	}

	query := `
		INSERT INTO routes (id, source_id, destination_id, enabled, filters, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, source_id, destination_id, enabled, filters, created_at, updated_at;
	`
	var created Route
	err := s.db.QueryRowContext(
		ctx,
		query,
		r.ID,
		r.SourceID,
		r.DestinationID,
		r.Enabled,
		r.Filters,
		r.CreatedAt,
		r.UpdatedAt,
	).Scan(
		&created.ID,
		&created.SourceID,
		&created.DestinationID,
		&created.Enabled,
		&created.Filters,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create route: %w", err)
	}
	return &created, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Route, error) {
	query := `
		SELECT id, source_id, destination_id, enabled, filters, created_at, updated_at
		FROM routes
		WHERE id = $1;
	`
	var r Route
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&r.ID,
		&r.SourceID,
		&r.DestinationID,
		&r.Enabled,
		&r.Filters,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRouteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get route: %w", err)
	}
	return &r, nil
}

func (s *PostgresStore) GetBySourceID(ctx context.Context, sourceID string) ([]*Route, error) {
	query := `
		SELECT id, source_id, destination_id, enabled, filters, created_at, updated_at
		FROM routes
		WHERE source_id = $1 AND enabled = true
		ORDER BY created_at ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get routes by source id: %w", err)
	}
	defer rows.Close()

	var list []*Route
	for rows.Next() {
		var r Route
		if err := rows.Scan(
			&r.ID,
			&r.SourceID,
			&r.DestinationID,
			&r.Enabled,
			&r.Filters,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan route row: %w", err)
		}
		list = append(list, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return list, nil
}

func (s *PostgresStore) Update(ctx context.Context, r *Route) (*Route, error) {
	r.UpdatedAt = time.Now().UTC()
	if len(r.Filters) == 0 {
		r.Filters = json.RawMessage("{}")
	}

	query := `
		UPDATE routes
		SET source_id = $2, destination_id = $3, enabled = $4, filters = $5, updated_at = $6
		WHERE id = $1
		RETURNING id, source_id, destination_id, enabled, filters, created_at, updated_at;
	`
	var updated Route
	err := s.db.QueryRowContext(
		ctx,
		query,
		r.ID,
		r.SourceID,
		r.DestinationID,
		r.Enabled,
		r.Filters,
		r.UpdatedAt,
	).Scan(
		&updated.ID,
		&updated.SourceID,
		&updated.DestinationID,
		&updated.Enabled,
		&updated.Filters,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRouteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update route: %w", err)
	}
	return &updated, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM routes WHERE id = $1;`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}
	if rows == 0 {
		return ErrRouteNotFound
	}
	return nil
}

func (s *PostgresStore) List(ctx context.Context) ([]*Route, error) {
	query := `
		SELECT id, source_id, destination_id, enabled, filters, created_at, updated_at
		FROM routes
		ORDER BY created_at DESC;
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}
	defer rows.Close()

	var list []*Route
	for rows.Next() {
		var r Route
		if err := rows.Scan(
			&r.ID,
			&r.SourceID,
			&r.DestinationID,
			&r.Enabled,
			&r.Filters,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan route row: %w", err)
		}
		list = append(list, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return list, nil
}
